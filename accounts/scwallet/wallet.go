















package scwallet

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	pcsc "github.com/gballet/go-libpcsclite"
	"github.com/status-im/keycard-go/derivationpath"
)




var ErrPairingPasswordNeeded = errors.New("smartcard: pairing password needed")




var ErrPINNeeded = errors.New("smartcard: pin needed")





var ErrPINUnblockNeeded = errors.New("smartcard: pin unblock needed")



var ErrAlreadyOpen = errors.New("smartcard: already open")



var ErrPubkeyMismatch = errors.New("smartcard: recovered public key mismatch")

var (
	appletAID = []byte{0xA0, 0x00, 0x00, 0x08, 0x04, 0x00, 0x01, 0x01, 0x01}
	
	DerivationSignatureHash = sha256.Sum256(common.Hash{}.Bytes())
)


const (
	claISO7816  = 0
	claSCWallet = 0x80

	insSelect      = 0xA4
	insGetResponse = 0xC0
	sw1GetResponse = 0x61
	sw1Ok          = 0x90

	insVerifyPin  = 0x20
	insUnblockPin = 0x22
	insExportKey  = 0xC2
	insSign       = 0xC0
	insLoadKey    = 0xD0
	insDeriveKey  = 0xD1
	insStatus     = 0xF2
)


const (
	P1DeriveKeyFromMaster  = uint8(0x00)
	P1DeriveKeyFromParent  = uint8(0x01)
	P1DeriveKeyFromCurrent = uint8(0x10)
	statusP1WalletStatus   = uint8(0x00)
	statusP1Path           = uint8(0x01)
	signP1PrecomputedHash  = uint8(0x01)
	signP2OnlyBlock        = uint8(0x81)
	exportP1Any            = uint8(0x00)
	exportP2Pubkey         = uint8(0x01)
)



const selfDeriveThrottling = time.Second


type Wallet struct {
	Hub       *Hub   
	PublicKey []byte 

	lock    sync.Mutex 
	card    *pcsc.Card 
	session *Session   
	log     log.Logger 

	deriveNextPaths []accounts.DerivationPath 
	deriveNextAddrs []common.Address          
	deriveChain     ethereum.ChainStateReader 
	deriveReq       chan chan struct{}        
	deriveQuit      chan chan error           
}


func NewWallet(hub *Hub, card *pcsc.Card) *Wallet {
	wallet := &Wallet{
		Hub:  hub,
		card: card,
	}
	return wallet
}




func transmit(card *pcsc.Card, command *commandAPDU) (*responseAPDU, error) {
	data, err := command.serialize()
	if err != nil {
		return nil, err
	}

	responseData, _, err := card.Transmit(data)
	if err != nil {
		return nil, err
	}

	response := new(responseAPDU)
	if err = response.deserialize(responseData); err != nil {
		return nil, err
	}

	
	if response.Sw1 == sw1GetResponse && (command.Cla != claISO7816 || command.Ins != insGetResponse) {
		return transmit(card, &commandAPDU{
			Cla:  claISO7816,
			Ins:  insGetResponse,
			P1:   0,
			P2:   0,
			Data: nil,
			Le:   response.Sw2,
		})
	}

	if response.Sw1 != sw1Ok {
		return nil, fmt.Errorf("unexpected insecure response status Cla=0x%x, Ins=0x%x, Sw=0x%x%x", command.Cla, command.Ins, response.Sw1, response.Sw2)
	}

	return response, nil
}



type applicationInfo struct {
	InstanceUID []byte `asn1:"tag:15"`
	PublicKey   []byte `asn1:"tag:0"`
}



func (w *Wallet) connect() error {
	w.lock.Lock()
	defer w.lock.Unlock()

	appinfo, err := w.doselect()
	if err != nil {
		return err
	}

	channel, err := NewSecureChannelSession(w.card, appinfo.PublicKey)
	if err != nil {
		return err
	}

	w.PublicKey = appinfo.PublicKey
	w.log = log.New("url", w.URL())
	w.session = &Session{
		Wallet:  w,
		Channel: channel,
	}
	return nil
}


func (w *Wallet) doselect() (*applicationInfo, error) {
	response, err := transmit(w.card, &commandAPDU{
		Cla:  claISO7816,
		Ins:  insSelect,
		P1:   4,
		P2:   0,
		Data: appletAID,
	})
	if err != nil {
		return nil, err
	}

	appinfo := new(applicationInfo)
	if _, err := asn1.UnmarshalWithParams(response.Data, appinfo, "tag:4"); err != nil {
		return nil, err
	}
	return appinfo, nil
}


func (w *Wallet) ping() error {
	w.lock.Lock()
	defer w.lock.Unlock()

	
	if !w.session.paired() {
		return nil
	}
	if _, err := w.session.walletStatus(); err != nil {
		return err
	}
	return nil
}


func (w *Wallet) release() error {
	if w.session != nil {
		return w.session.release()
	}
	return nil
}



func (w *Wallet) pair(puk []byte) error {
	if w.session.paired() {
		return fmt.Errorf("wallet already paired")
	}
	pairing, err := w.session.pair(puk)
	if err != nil {
		return err
	}
	if err = w.Hub.setPairing(w, &pairing); err != nil {
		return err
	}
	return w.session.authenticate(pairing)
}


func (w *Wallet) Unpair(pin []byte) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if !w.session.paired() {
		return fmt.Errorf("wallet %x not paired", w.PublicKey)
	}
	if err := w.session.verifyPin(pin); err != nil {
		return fmt.Errorf("failed to verify pin: %s", err)
	}
	if err := w.session.unpair(); err != nil {
		return fmt.Errorf("failed to unpair: %s", err)
	}
	if err := w.Hub.setPairing(w, nil); err != nil {
		return err
	}
	return nil
}




func (w *Wallet) URL() accounts.URL {
	return accounts.URL{
		Scheme: w.Hub.scheme,
		Path:   fmt.Sprintf("%x", w.PublicKey[1:5]), 
	}
}




func (w *Wallet) Status() (string, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	
	if !w.session.paired() {
		return "Unpaired, waiting for pairing password", nil
	}
	
	status, err := w.session.walletStatus()
	if err != nil {
		return fmt.Sprintf("Failed: %v", err), err
	}
	switch {
	case !w.session.verified && status.PinRetryCount == 0 && status.PukRetryCount == 0:
		return "Bricked, waiting for full wipe", nil
	case !w.session.verified && status.PinRetryCount == 0:
		return fmt.Sprintf("Blocked, waiting for PUK (%d attempts left) and new PIN", status.PukRetryCount), nil
	case !w.session.verified:
		return fmt.Sprintf("Locked, waiting for PIN (%d attempts left)", status.PinRetryCount), nil
	case !status.Initialized:
		return "Empty, waiting for initialization", nil
	default:
		return "Online", nil
	}
}












func (w *Wallet) Open(passphrase string) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	
	if w.session.verified {
		return ErrAlreadyOpen
	}
	
	
	if !w.session.paired() {
		
		if pairing := w.Hub.pairing(w); pairing != nil {
			if err := w.session.authenticate(*pairing); err != nil {
				return fmt.Errorf("failed to authenticate card %x: %s", w.PublicKey[:4], err)
			}
			
		} else {
			
			if passphrase == "" {
				return ErrPairingPasswordNeeded
			}
			
			if err := w.pair([]byte(passphrase)); err != nil {
				return err
			}
			
			
			
			passphrase = ""
		}
	}
	
	
	status, err := w.session.walletStatus()
	if err != nil {
		return err
	}
	
	switch {
	case passphrase == "" && status.PinRetryCount > 0:
		return ErrPINNeeded
	case passphrase == "":
		return ErrPINUnblockNeeded
	case status.PinRetryCount > 0:
		if !regexp.MustCompile(`^[0-9]{6,}$`).MatchString(passphrase) {
			w.log.Error("PIN needs to be at least 6 digits")
			return ErrPINNeeded
		}
		if err := w.session.verifyPin([]byte(passphrase)); err != nil {
			return err
		}
	default:
		if !regexp.MustCompile(`^[0-9]{12,}$`).MatchString(passphrase) {
			w.log.Error("PUK needs to be at least 12 digits")
			return ErrPINUnblockNeeded
		}
		if err := w.session.unblockPin([]byte(passphrase)); err != nil {
			return err
		}
	}
	
	w.deriveReq = make(chan chan struct{})
	w.deriveQuit = make(chan chan error)

	go w.selfDerive()

	
	go w.Hub.updateFeed.Send(accounts.WalletEvent{Wallet: w, Kind: accounts.WalletOpened})

	return nil
}


func (w *Wallet) Close() error {
	
	w.lock.Lock()
	dQuit := w.deriveQuit
	w.lock.Unlock()

	
	var derr error
	if dQuit != nil {
		errc := make(chan error)
		dQuit <- errc
		derr = <-errc 
	}
	
	w.lock.Lock()
	defer w.lock.Unlock()

	w.deriveQuit = nil
	w.deriveReq = nil

	if err := w.release(); err != nil {
		return err
	}
	return derr
}



func (w *Wallet) selfDerive() {
	w.log.Debug("Smart card wallet self-derivation started")
	defer w.log.Debug("Smart card wallet self-derivation stopped")

	
	var (
		reqc chan struct{}
		errc chan error
		err  error
	)
	for errc == nil && err == nil {
		
		select {
		case errc = <-w.deriveQuit:
			
			continue
		case reqc = <-w.deriveReq:
			
		}
		
		w.lock.Lock()
		if w.session == nil || w.deriveChain == nil {
			w.lock.Unlock()
			reqc <- struct{}{}
			continue
		}
		pairing := w.Hub.pairing(w)

		
		var (
			paths   []accounts.DerivationPath
			nextAcc accounts.Account

			nextPaths = append([]accounts.DerivationPath{}, w.deriveNextPaths...)
			nextAddrs = append([]common.Address{}, w.deriveNextAddrs...)

			context = context.Background()
		)
		for i := 0; i < len(nextAddrs); i++ {
			for empty := false; !empty; {
				
				if nextAddrs[i] == (common.Address{}) {
					if nextAcc, err = w.session.derive(nextPaths[i]); err != nil {
						w.log.Warn("Smartcard wallet account derivation failed", "err", err)
						break
					}
					nextAddrs[i] = nextAcc.Address
				}
				
				var (
					balance *big.Int
					nonce   uint64
				)
				balance, err = w.deriveChain.BalanceAt(context, nextAddrs[i], nil)
				if err != nil {
					w.log.Warn("Smartcard wallet balance retrieval failed", "err", err)
					break
				}
				nonce, err = w.deriveChain.NonceAt(context, nextAddrs[i], nil)
				if err != nil {
					w.log.Warn("Smartcard wallet nonce retrieval failed", "err", err)
					break
				}
				
				if balance.Sign() == 0 && nonce == 0 {
					empty = true
					if i < len(nextAddrs)-1 {
						break
					}
				}
				
				path := make(accounts.DerivationPath, len(nextPaths[i]))
				copy(path[:], nextPaths[i][:])
				paths = append(paths, path)

				
				if _, known := pairing.Accounts[nextAddrs[i]]; !known || !empty || nextAddrs[i] != w.deriveNextAddrs[i] {
					w.log.Info("Smartcard wallet discovered new account", "address", nextAddrs[i], "path", path, "balance", balance, "nonce", nonce)
				}
				pairing.Accounts[nextAddrs[i]] = path

				
				if !empty {
					nextAddrs[i] = common.Address{}
					nextPaths[i][len(nextPaths[i])-1]++
				}
			}
		}
		
		if len(paths) > 0 {
			err = w.Hub.setPairing(w, pairing)
		}
		
		w.deriveNextAddrs = nextAddrs
		w.deriveNextPaths = nextPaths

		
		w.lock.Unlock()

		
		reqc <- struct{}{}
		if err == nil {
			select {
			case errc = <-w.deriveQuit:
				
			case <-time.After(selfDeriveThrottling):
				
			}
		}
	}
	
	if err != nil {
		w.log.Debug("Smartcard wallet self-derivation failed", "err", err)
		errc = <-w.deriveQuit
	}
	errc <- err
}




func (w *Wallet) Accounts() []accounts.Account {
	
	reqc := make(chan struct{}, 1)
	select {
	case w.deriveReq <- reqc:
		
		<-reqc
	default:
		
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	if pairing := w.Hub.pairing(w); pairing != nil {
		ret := make([]accounts.Account, 0, len(pairing.Accounts))
		for address, path := range pairing.Accounts {
			ret = append(ret, w.makeAccount(address, path))
		}
		sort.Sort(accounts.AccountsByURL(ret))
		return ret
	}
	return nil
}

func (w *Wallet) makeAccount(address common.Address, path accounts.DerivationPath) accounts.Account {
	return accounts.Account{
		Address: address,
		URL: accounts.URL{
			Scheme: w.Hub.scheme,
			Path:   fmt.Sprintf("%x/%s", w.PublicKey[1:3], path.String()),
		},
	}
}


func (w *Wallet) Contains(account accounts.Account) bool {
	if pairing := w.Hub.pairing(w); pairing != nil {
		_, ok := pairing.Accounts[account.Address]
		return ok
	}
	return false
}


func (w *Wallet) Initialize(seed []byte) error {
	go w.selfDerive()
	
	
	return w.session.initialize(seed)
}




func (w *Wallet) Derive(path accounts.DerivationPath, pin bool) (accounts.Account, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	account, err := w.session.derive(path)
	if err != nil {
		return accounts.Account{}, err
	}

	if pin {
		pairing := w.Hub.pairing(w)
		pairing.Accounts[account.Address] = path
		if err := w.Hub.setPairing(w, pairing); err != nil {
			return accounts.Account{}, err
		}
	}

	return account, nil
}















func (w *Wallet) SelfDerive(bases []accounts.DerivationPath, chain ethereum.ChainStateReader) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.deriveNextPaths = make([]accounts.DerivationPath, len(bases))
	for i, base := range bases {
		w.deriveNextPaths[i] = make(accounts.DerivationPath, len(base))
		copy(w.deriveNextPaths[i][:], base[:])
	}
	w.deriveNextAddrs = make([]common.Address, len(bases))
	w.deriveChain = chain
}












func (w *Wallet) SignData(account accounts.Account, mimeType string, data []byte) ([]byte, error) {
	return w.signHash(account, crypto.Keccak256(data))
}

func (w *Wallet) signHash(account accounts.Account, hash []byte) ([]byte, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	path, err := w.findAccountPath(account)
	if err != nil {
		return nil, err
	}

	return w.session.sign(path, hash)
}












func (w *Wallet) SignTx(account accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.NewEIP155Signer(chainID)
	hash := signer.Hash(tx)
	sig, err := w.signHash(account, hash[:])
	if err != nil {
		return nil, err
	}
	return tx.WithSignature(signer, sig)
}






func (w *Wallet) SignDataWithPassphrase(account accounts.Account, passphrase, mimeType string, data []byte) ([]byte, error) {
	return w.signHashWithPassphrase(account, passphrase, crypto.Keccak256(data))
}

func (w *Wallet) signHashWithPassphrase(account accounts.Account, passphrase string, hash []byte) ([]byte, error) {
	if !w.session.verified {
		if err := w.Open(passphrase); err != nil {
			return nil, err
		}
	}

	return w.signHash(account, hash)
}












func (w *Wallet) SignText(account accounts.Account, text []byte) ([]byte, error) {
	return w.signHash(account, accounts.TextHash(text))
}



func (w *Wallet) SignTextWithPassphrase(account accounts.Account, passphrase string, text []byte) ([]byte, error) {
	return w.signHashWithPassphrase(account, passphrase, crypto.Keccak256(accounts.TextHash(text)))
}






func (w *Wallet) SignTxWithPassphrase(account accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	if !w.session.verified {
		if err := w.Open(passphrase); err != nil {
			return nil, err
		}
	}
	return w.SignTx(account, tx, chainID)
}




func (w *Wallet) findAccountPath(account accounts.Account) (accounts.DerivationPath, error) {
	pairing := w.Hub.pairing(w)
	if path, ok := pairing.Accounts[account.Address]; ok {
		return path, nil
	}

	
	if account.URL.Scheme != w.Hub.scheme {
		return nil, fmt.Errorf("scheme %s does not match wallet scheme %s", account.URL.Scheme, w.Hub.scheme)
	}

	parts := strings.SplitN(account.URL.Path, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid URL format: %s", account.URL)
	}

	if parts[0] != fmt.Sprintf("%x", w.PublicKey[1:3]) {
		return nil, fmt.Errorf("URL %s is not for this wallet", account.URL)
	}

	return accounts.ParseDerivationPath(parts[1])
}


type Session struct {
	Wallet   *Wallet               
	Channel  *SecureChannelSession 
	verified bool                  
}


func (s *Session) pair(secret []byte) (smartcardPairing, error) {
	err := s.Channel.Pair(secret)
	if err != nil {
		return smartcardPairing{}, err
	}

	return smartcardPairing{
		PublicKey:    s.Wallet.PublicKey,
		PairingIndex: s.Channel.PairingIndex,
		PairingKey:   s.Channel.PairingKey,
		Accounts:     make(map[common.Address]accounts.DerivationPath),
	}, nil
}


func (s *Session) unpair() error {
	if !s.verified {
		return fmt.Errorf("unpair requires that the PIN be verified")
	}
	return s.Channel.Unpair()
}


func (s *Session) verifyPin(pin []byte) error {
	if _, err := s.Channel.transmitEncrypted(claSCWallet, insVerifyPin, 0, 0, pin); err != nil {
		return err
	}
	s.verified = true
	return nil
}



func (s *Session) unblockPin(pukpin []byte) error {
	if _, err := s.Channel.transmitEncrypted(claSCWallet, insUnblockPin, 0, 0, pukpin); err != nil {
		return err
	}
	s.verified = true
	return nil
}


func (s *Session) release() error {
	return s.Wallet.card.Disconnect(pcsc.LeaveCard)
}


func (s *Session) paired() bool {
	return s.Channel.PairingKey != nil
}


func (s *Session) authenticate(pairing smartcardPairing) error {
	if !bytes.Equal(s.Wallet.PublicKey, pairing.PublicKey) {
		return fmt.Errorf("cannot pair using another wallet's pairing; %x != %x", s.Wallet.PublicKey, pairing.PublicKey)
	}
	s.Channel.PairingKey = pairing.PairingKey
	s.Channel.PairingIndex = pairing.PairingIndex
	return s.Channel.Open()
}


type walletStatus struct {
	PinRetryCount int  
	PukRetryCount int  
	Initialized   bool 
}


func (s *Session) walletStatus() (*walletStatus, error) {
	response, err := s.Channel.transmitEncrypted(claSCWallet, insStatus, statusP1WalletStatus, 0, nil)
	if err != nil {
		return nil, err
	}

	status := new(walletStatus)
	if _, err := asn1.UnmarshalWithParams(response.Data, status, "tag:3"); err != nil {
		return nil, err
	}
	return status, nil
}



func (s *Session) derivationPath() (accounts.DerivationPath, error) {
	response, err := s.Channel.transmitEncrypted(claSCWallet, insStatus, statusP1Path, 0, nil)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewReader(response.Data)
	path := make(accounts.DerivationPath, len(response.Data)/4)
	return path, binary.Read(buf, binary.BigEndian, &path)
}


type initializeData struct {
	PublicKey  []byte `asn1:"tag:0"`
	PrivateKey []byte `asn1:"tag:1"`
	ChainCode  []byte `asn1:"tag:2"`
}


func (s *Session) initialize(seed []byte) error {
	
	
	status, err := s.Wallet.Status()
	if err != nil {
		return err
	}
	if status == "Online" {
		return fmt.Errorf("card is already initialized, cowardly refusing to proceed")
	}

	s.Wallet.lock.Lock()
	defer s.Wallet.lock.Unlock()

	
	mac := hmac.New(sha512.New, []byte("Bitcoin seed"))
	mac.Write(seed)
	seed = mac.Sum(nil)

	key, err := crypto.ToECDSA(seed[:32])
	if err != nil {
		return err
	}

	id := initializeData{}
	id.PublicKey = crypto.FromECDSAPub(&key.PublicKey)
	id.PrivateKey = seed[:32]
	id.ChainCode = seed[32:]
	data, err := asn1.Marshal(id)
	if err != nil {
		return err
	}

	
	data[0] = 0xA1

	_, err = s.Channel.transmitEncrypted(claSCWallet, insLoadKey, 0x02, 0, data)
	return err
}


func (s *Session) derive(path accounts.DerivationPath) (accounts.Account, error) {
	startingPoint, path, err := derivationpath.Decode(path.String())
	if err != nil {
		return accounts.Account{}, err
	}

	var p1 uint8
	switch startingPoint {
	case derivationpath.StartingPointMaster:
		p1 = P1DeriveKeyFromMaster
	case derivationpath.StartingPointParent:
		p1 = P1DeriveKeyFromParent
	case derivationpath.StartingPointCurrent:
		p1 = P1DeriveKeyFromCurrent
	default:
		return accounts.Account{}, fmt.Errorf("invalid startingPoint %d", startingPoint)
	}

	data := new(bytes.Buffer)
	for _, segment := range path {
		if err := binary.Write(data, binary.BigEndian, segment); err != nil {
			return accounts.Account{}, err
		}
	}

	_, err = s.Channel.transmitEncrypted(claSCWallet, insDeriveKey, p1, 0, data.Bytes())
	if err != nil {
		return accounts.Account{}, err
	}

	response, err := s.Channel.transmitEncrypted(claSCWallet, insSign, 0, 0, DerivationSignatureHash[:])
	if err != nil {
		return accounts.Account{}, err
	}

	sigdata := new(signatureData)
	if _, err := asn1.UnmarshalWithParams(response.Data, sigdata, "tag:0"); err != nil {
		return accounts.Account{}, err
	}
	rbytes, sbytes := sigdata.Signature.R.Bytes(), sigdata.Signature.S.Bytes()
	sig := make([]byte, 65)
	copy(sig[32-len(rbytes):32], rbytes)
	copy(sig[64-len(sbytes):64], sbytes)

	if err := confirmPublicKey(sig, sigdata.PublicKey); err != nil {
		return accounts.Account{}, err
	}
	pub, err := crypto.UnmarshalPubkey(sigdata.PublicKey)
	if err != nil {
		return accounts.Account{}, err
	}
	return s.Wallet.makeAccount(crypto.PubkeyToAddress(*pub), path), nil
}



type keyExport struct {
	PublicKey  []byte `asn1:"tag:0"`
	PrivateKey []byte `asn1:"tag:1,optional"`
}



func (s *Session) publicKey() ([]byte, error) {
	response, err := s.Channel.transmitEncrypted(claSCWallet, insExportKey, exportP1Any, exportP2Pubkey, nil)
	if err != nil {
		return nil, err
	}
	keys := new(keyExport)
	if _, err := asn1.UnmarshalWithParams(response.Data, keys, "tag:1"); err != nil {
		return nil, err
	}
	return keys.PublicKey, nil
}



type signatureData struct {
	PublicKey []byte `asn1:"tag:0"`
	Signature struct {
		R *big.Int
		S *big.Int
	}
}



func (s *Session) sign(path accounts.DerivationPath, hash []byte) ([]byte, error) {
	startTime := time.Now()
	_, err := s.derive(path)
	if err != nil {
		return nil, err
	}
	deriveTime := time.Now()

	response, err := s.Channel.transmitEncrypted(claSCWallet, insSign, signP1PrecomputedHash, signP2OnlyBlock, hash)
	if err != nil {
		return nil, err
	}
	sigdata := new(signatureData)
	if _, err := asn1.UnmarshalWithParams(response.Data, sigdata, "tag:0"); err != nil {
		return nil, err
	}
	
	rbytes, sbytes := sigdata.Signature.R.Bytes(), sigdata.Signature.S.Bytes()
	sig := make([]byte, 65)
	copy(sig[32-len(rbytes):32], rbytes)
	copy(sig[64-len(sbytes):64], sbytes)

	
	sig, err = makeRecoverableSignature(hash, sig, sigdata.PublicKey)
	if err != nil {
		return nil, err
	}
	log.Debug("Signed using smartcard", "deriveTime", deriveTime.Sub(startTime), "signingTime", time.Since(deriveTime))

	return sig, nil
}


func confirmPublicKey(sig, pubkey []byte) error {
	_, err := makeRecoverableSignature(DerivationSignatureHash[:], sig, pubkey)
	return err
}



func makeRecoverableSignature(hash, sig, expectedPubkey []byte) ([]byte, error) {
	var libraryError error
	for v := 0; v < 2; v++ {
		sig[64] = byte(v)
		if pubkey, err := crypto.Ecrecover(hash, sig); err == nil {
			if bytes.Equal(pubkey, expectedPubkey) {
				return sig, nil
			}
		} else {
			libraryError = err
		}
	}
	if libraryError != nil {
		return nil, libraryError
	}
	return nil, ErrPubkeyMismatch
}

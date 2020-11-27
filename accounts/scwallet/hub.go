































package scwallet

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	pcsc "github.com/gballet/go-libpcsclite"
)


const Scheme = "keycard"



const refreshCycle = time.Second


const refreshThrottling = 500 * time.Millisecond



type smartcardPairing struct {
	PublicKey    []byte                                     `json:"publicKey"`
	PairingIndex uint8                                      `json:"pairingIndex"`
	PairingKey   []byte                                     `json:"pairingKey"`
	Accounts     map[common.Address]accounts.DerivationPath `json:"accounts"`
}


type Hub struct {
	scheme string 

	context  *pcsc.Client
	datadir  string
	pairings map[string]smartcardPairing

	refreshed   time.Time               
	wallets     map[string]*Wallet      
	updateFeed  event.Feed              
	updateScope event.SubscriptionScope 
	updating    bool                    

	quit chan chan error

	stateLock sync.RWMutex 
}

func (hub *Hub) readPairings() error {
	hub.pairings = make(map[string]smartcardPairing)
	pairingFile, err := os.Open(filepath.Join(hub.datadir, "smartcards.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	pairingData, err := ioutil.ReadAll(pairingFile)
	if err != nil {
		return err
	}
	var pairings []smartcardPairing
	if err := json.Unmarshal(pairingData, &pairings); err != nil {
		return err
	}

	for _, pairing := range pairings {
		hub.pairings[string(pairing.PublicKey)] = pairing
	}
	return nil
}

func (hub *Hub) writePairings() error {
	pairingFile, err := os.OpenFile(filepath.Join(hub.datadir, "smartcards.json"), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	defer pairingFile.Close()

	pairings := make([]smartcardPairing, 0, len(hub.pairings))
	for _, pairing := range hub.pairings {
		pairings = append(pairings, pairing)
	}

	pairingData, err := json.Marshal(pairings)
	if err != nil {
		return err
	}

	if _, err := pairingFile.Write(pairingData); err != nil {
		return err
	}

	return nil
}

func (hub *Hub) pairing(wallet *Wallet) *smartcardPairing {
	if pairing, ok := hub.pairings[string(wallet.PublicKey)]; ok {
		return &pairing
	}
	return nil
}

func (hub *Hub) setPairing(wallet *Wallet, pairing *smartcardPairing) error {
	if pairing == nil {
		delete(hub.pairings, string(wallet.PublicKey))
	} else {
		hub.pairings[string(wallet.PublicKey)] = *pairing
	}
	return hub.writePairings()
}


func NewHub(daemonPath string, scheme string, datadir string) (*Hub, error) {
	context, err := pcsc.EstablishContext(daemonPath, pcsc.ScopeSystem)
	if err != nil {
		return nil, err
	}
	hub := &Hub{
		scheme:  scheme,
		context: context,
		datadir: datadir,
		wallets: make(map[string]*Wallet),
		quit:    make(chan chan error),
	}
	if err := hub.readPairings(); err != nil {
		return nil, err
	}
	hub.refreshWallets()
	return hub, nil
}



func (hub *Hub) Wallets() []accounts.Wallet {
	
	hub.refreshWallets()

	hub.stateLock.RLock()
	defer hub.stateLock.RUnlock()

	cpy := make([]accounts.Wallet, 0, len(hub.wallets))
	for _, wallet := range hub.wallets {
		cpy = append(cpy, wallet)
	}
	sort.Sort(accounts.WalletsByURL(cpy))
	return cpy
}



func (hub *Hub) refreshWallets() {
	
	hub.stateLock.RLock()
	elapsed := time.Since(hub.refreshed)
	hub.stateLock.RUnlock()

	if elapsed < refreshThrottling {
		return
	}
	
	readers, err := hub.context.ListReaders()
	if err != nil {
		
		
		
		if err.Error() != "scard: Cannot find a smart card reader." {
			log.Error("Failed to enumerate smart card readers", "err", err)
			return
		}
	}
	
	hub.stateLock.Lock()

	events := []accounts.WalletEvent{}
	seen := make(map[string]struct{})

	for _, reader := range readers {
		
		seen[reader] = struct{}{}

		
		if wallet, ok := hub.wallets[reader]; ok {
			if err := wallet.ping(); err == nil {
				continue
			}
			wallet.Close()
			events = append(events, accounts.WalletEvent{Wallet: wallet, Kind: accounts.WalletDropped})
			delete(hub.wallets, reader)
		}
		
		card, err := hub.context.Connect(reader, pcsc.ShareShared, pcsc.ProtocolAny)
		if err != nil {
			log.Debug("Failed to open smart card", "reader", reader, "err", err)
			continue
		}
		wallet := NewWallet(hub, card)
		if err = wallet.connect(); err != nil {
			log.Debug("Failed to connect to smart card", "reader", reader, "err", err)
			card.Disconnect(pcsc.LeaveCard)
			continue
		}
		
		hub.wallets[reader] = wallet
		events = append(events, accounts.WalletEvent{Wallet: wallet, Kind: accounts.WalletArrived})
	}
	
	for reader, wallet := range hub.wallets {
		if _, ok := seen[reader]; !ok {
			wallet.Close()
			events = append(events, accounts.WalletEvent{Wallet: wallet, Kind: accounts.WalletDropped})
			delete(hub.wallets, reader)
		}
	}
	hub.refreshed = time.Now()
	hub.stateLock.Unlock()

	for _, event := range events {
		hub.updateFeed.Send(event)
	}
}



func (hub *Hub) Subscribe(sink chan<- accounts.WalletEvent) event.Subscription {
	
	hub.stateLock.Lock()
	defer hub.stateLock.Unlock()

	
	sub := hub.updateScope.Track(hub.updateFeed.Subscribe(sink))

	
	if !hub.updating {
		hub.updating = true
		go hub.updater()
	}
	return sub
}



func (hub *Hub) updater() {
	for {
		
		
		time.Sleep(refreshCycle)

		
		hub.refreshWallets()

		
		hub.stateLock.Lock()
		if hub.updateScope.Count() == 0 {
			hub.updating = false
			hub.stateLock.Unlock()
			return
		}
		hub.stateLock.Unlock()
	}
}

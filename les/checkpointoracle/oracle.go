


















package checkpointoracle

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/checkpointoracle"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)




type CheckpointOracle struct {
	config   *params.CheckpointOracleConfig
	contract *checkpointoracle.CheckpointOracle

	running  int32                                 
	getLocal func(uint64) params.TrustedCheckpoint 

	checkMu              sync.Mutex                
	lastCheckTime        time.Time                 
	lastCheckPoint       *params.TrustedCheckpoint 
	lastCheckPointHeight uint64                    
}


func New(config *params.CheckpointOracleConfig, getLocal func(uint64) params.TrustedCheckpoint) *CheckpointOracle {
	return &CheckpointOracle{
		config:   config,
		getLocal: getLocal,
	}
}



func (oracle *CheckpointOracle) Start(backend bind.ContractBackend) {
	contract, err := checkpointoracle.NewCheckpointOracle(oracle.config.Address, backend)
	if err != nil {
		log.Error("Oracle contract binding failed", "err", err)
		return
	}
	if !atomic.CompareAndSwapInt32(&oracle.running, 0, 1) {
		log.Error("Already bound and listening to registrar")
		return
	}
	oracle.contract = contract
}


func (oracle *CheckpointOracle) IsRunning() bool {
	return atomic.LoadInt32(&oracle.running) == 1
}


func (oracle *CheckpointOracle) Contract() *checkpointoracle.CheckpointOracle {
	return oracle.contract
}



func (oracle *CheckpointOracle) StableCheckpoint() (*params.TrustedCheckpoint, uint64) {
	oracle.checkMu.Lock()
	defer oracle.checkMu.Unlock()
	if time.Since(oracle.lastCheckTime) < 1*time.Minute {
		return oracle.lastCheckPoint, oracle.lastCheckPointHeight
	}
	
	
	latest, hash, height, err := oracle.contract.Contract().GetLatestCheckpoint(nil)
	oracle.lastCheckTime = time.Now()
	if err != nil || (latest == 0 && hash == [32]byte{}) {
		oracle.lastCheckPointHeight = 0
		oracle.lastCheckPoint = nil
		return oracle.lastCheckPoint, oracle.lastCheckPointHeight
	}
	local := oracle.getLocal(latest)

	
	
	
	
	
	
	
	if local.HashEqual(hash) {
		oracle.lastCheckPointHeight = height.Uint64()
		oracle.lastCheckPoint = &local
		return oracle.lastCheckPoint, oracle.lastCheckPointHeight
	}
	return nil, 0
}



func (oracle *CheckpointOracle) VerifySigners(index uint64, hash [32]byte, signatures [][]byte) (bool, []common.Address) {
	
	if len(signatures) < int(oracle.config.Threshold) {
		return false, nil
	}
	var (
		signers []common.Address
		checked = make(map[common.Address]struct{})
	)
	for i := 0; i < len(signatures); i++ {
		if len(signatures[i]) != 65 {
			continue
		}
		
		
		
		
		
		
		
		
		
		
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, index)
		data := append([]byte{0x19, 0x00}, append(oracle.config.Address.Bytes(), append(buf, hash[:]...)...)...)
		signatures[i][64] -= 27 
		pubkey, err := crypto.Ecrecover(crypto.Keccak256(data), signatures[i])
		if err != nil {
			return false, nil
		}
		var signer common.Address
		copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])
		if _, exist := checked[signer]; exist {
			continue
		}
		for _, s := range oracle.config.Signers {
			if s == signer {
				signers = append(signers, signer)
				checked[signer] = struct{}{}
			}
		}
	}
	threshold := oracle.config.Threshold
	if uint64(len(signers)) < threshold {
		log.Warn("Not enough signers to approve checkpoint", "signers", len(signers), "threshold", threshold)
		return false, nil
	}
	return true, signers
}

















package txfetcher

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/fetcher"
)

var (
	peers []string
	txs   []*types.Transaction
)

func init() {
	
	rand := rand.New(rand.NewSource(0x3a29))

	peers = make([]string, 10)
	for i := 0; i < len(peers); i++ {
		peers[i] = fmt.Sprintf("Peer #%d", i)
	}
	txs = make([]*types.Transaction, 65536) 
	for i := 0; i < len(txs); i++ {
		txs[i] = types.NewTransaction(rand.Uint64(), common.Address{byte(rand.Intn(256))}, new(big.Int), 0, new(big.Int), nil)
	}
}

func Fuzz(input []byte) int {
	
	if len(input) > 16*1024 {
		return -1
	}
	r := bytes.NewReader(input)

	
	
	
	limit, err := r.ReadByte()
	if err != nil {
		return 0
	}
	switch limit % 4 {
	case 0:
		txs = txs[:4]
	case 1:
		txs = txs[:256]
	case 2:
		txs = txs[:4096]
	case 3:
		
	}
	
	clock := new(mclock.Simulated)
	rand := rand.New(rand.NewSource(0x3a29)) 

	f := fetcher.NewTxFetcherForTests(
		func(common.Hash) bool { return false },
		func(txs []*types.Transaction) []error {
			return make([]error, len(txs))
		},
		func(string, []common.Hash) error { return nil },
		clock, rand,
	)
	f.Start()
	defer f.Stop()

	
	for {
		
		cmd, err := r.ReadByte()
		if err != nil {
			return 0
		}
		switch cmd % 4 {
		case 0:
			
			
			
			
			peerIdx, err := r.ReadByte()
			if err != nil {
				return 0
			}
			peer := peers[int(peerIdx)%len(peers)]

			announceCnt, err := r.ReadByte()
			if err != nil {
				return 0
			}
			announce := int(announceCnt) % (2 * len(txs)) 

			var (
				announceIdxs = make([]int, announce)
				announces    = make([]common.Hash, announce)
			)
			for i := 0; i < len(announces); i++ {
				annBuf := make([]byte, 2)
				if n, err := r.Read(annBuf); err != nil || n != 2 {
					return 0
				}
				announceIdxs[i] = (int(annBuf[0])*256 + int(annBuf[1])) % len(txs)
				announces[i] = txs[announceIdxs[i]].Hash()
			}
			fmt.Println("Notify", peer, announceIdxs)
			if err := f.Notify(peer, announces); err != nil {
				panic(err)
			}

		case 1:
			
			
			
			
			peerIdx, err := r.ReadByte()
			if err != nil {
				return 0
			}
			peer := peers[int(peerIdx)%len(peers)]

			deliverCnt, err := r.ReadByte()
			if err != nil {
				return 0
			}
			deliver := int(deliverCnt) % (2 * len(txs)) 

			var (
				deliverIdxs = make([]int, deliver)
				deliveries  = make([]*types.Transaction, deliver)
			)
			for i := 0; i < len(deliveries); i++ {
				deliverBuf := make([]byte, 2)
				if n, err := r.Read(deliverBuf); err != nil || n != 2 {
					return 0
				}
				deliverIdxs[i] = (int(deliverBuf[0])*256 + int(deliverBuf[1])) % len(txs)
				deliveries[i] = txs[deliverIdxs[i]]
			}
			directFlag, err := r.ReadByte()
			if err != nil {
				return 0
			}
			direct := (directFlag % 2) == 0

			fmt.Println("Enqueue", peer, deliverIdxs, direct)
			if err := f.Enqueue(peer, deliveries, direct); err != nil {
				panic(err)
			}

		case 2:
			
			
			peerIdx, err := r.ReadByte()
			if err != nil {
				return 0
			}
			peer := peers[int(peerIdx)%len(peers)]

			fmt.Println("Drop", peer)
			if err := f.Drop(peer); err != nil {
				panic(err)
			}

		case 3:
			
			
			tickCnt, err := r.ReadByte()
			if err != nil {
				return 0
			}
			tick := time.Duration(tickCnt) * 100 * time.Millisecond

			fmt.Println("Sleep", tick)
			clock.Run(tick)
		}
	}
}

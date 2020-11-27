















package eth

import (
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
)


var DefaultFullGPOConfig = gasprice.Config{
	Blocks:     20,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}


var DefaultLightGPOConfig = gasprice.Config{
	Blocks:     2,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}


var DefaultConfig = Config{
	SyncMode: downloader.FastSync,
	Ethash: ethash.Config{
		CacheDir:         "ethash",
		CachesInMem:      2,
		CachesOnDisk:     3,
		CachesLockMmap:   false,
		DatasetsInMem:    1,
		DatasetsOnDisk:   2,
		DatasetsLockMmap: false,
	},
	NetworkId:               1,
	LightPeers:              100,
	UltraLightFraction:      75,
	DatabaseCache:           512,
	TrieCleanCache:          154,
	TrieCleanCacheJournal:   "triecache",
	TrieCleanCacheRejournal: 60 * time.Minute,
	TrieDirtyCache:          256,
	TrieTimeout:             60 * time.Minute,
	SnapshotCache:           102,
	Miner: miner.Config{
		GasFloor: 8000000,
		GasCeil:  8000000,
		GasPrice: big.NewInt(params.GWei),
		Recommit: 3 * time.Second,
	},
	TxPool:      core.DefaultTxPoolConfig,
	RPCGasCap:   25000000,
	GPO:         DefaultFullGPOConfig,
	RPCTxFeeCap: 1, 
}

func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "darwin" {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "Library", "Ethash")
	} else if runtime.GOOS == "windows" {
		localappdata := os.Getenv("LOCALAPPDATA")
		if localappdata != "" {
			DefaultConfig.Ethash.DatasetDir = filepath.Join(localappdata, "Ethash")
		} else {
			DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "AppData", "Local", "Ethash")
		}
	} else {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, ".ethash")
	}
}



type Config struct {
	
	
	Genesis *core.Genesis `toml:",omitempty"`

	
	NetworkId uint64 
	SyncMode  downloader.SyncMode

	
	
	DiscoveryURLs []string

	NoPruning  bool 
	NoPrefetch bool 

	TxLookupLimit uint64 `toml:",omitempty"` 

	
	Whitelist map[uint64]common.Hash `toml:"-"`

	
	LightServ    int  `toml:",omitempty"` 
	LightIngress int  `toml:",omitempty"` 
	LightEgress  int  `toml:",omitempty"` 
	LightPeers   int  `toml:",omitempty"` 
	LightNoPrune bool `toml:",omitempty"` 

	
	UltraLightServers      []string `toml:",omitempty"` 
	UltraLightFraction     int      `toml:",omitempty"` 
	UltraLightOnlyAnnounce bool     `toml:",omitempty"` 

	
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int
	DatabaseFreezer    string

	TrieCleanCache          int
	TrieCleanCacheJournal   string        `toml:",omitempty"` 
	TrieCleanCacheRejournal time.Duration `toml:",omitempty"` 
	TrieDirtyCache          int
	TrieTimeout             time.Duration
	SnapshotCache           int

	
	Miner miner.Config

	
	Ethash ethash.Config

	
	TxPool core.TxPoolConfig

	
	GPO gasprice.Config

	
	EnablePreimageRecording bool

	
	DocRoot string `toml:"-"`

	
	EWASMInterpreter string

	
	EVMInterpreter string

	
	RPCGasCap uint64 `toml:",omitempty"`

	
	
	RPCTxFeeCap float64 `toml:",omitempty"`

	
	Checkpoint *params.TrustedCheckpoint `toml:",omitempty"`

	
	CheckpointOracle *params.CheckpointOracleConfig `toml:",omitempty"`
}

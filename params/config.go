















package params

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)


var (
	MainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	RopstenGenesisHash = common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d")
	RinkebyGenesisHash = common.HexToHash("0x6341fd3daf94b748c72ced5a5b26028f2474f5f00d824504e4fa37a75767e177")
	GoerliGenesisHash  = common.HexToHash("0xbf7e331f7f7c1dd2e05159666b3bf8bc7a8a3a9eb1d518969eab529dd9b88c1a")
	YoloV1GenesisHash  = common.HexToHash("0xc3fd235071f24f93865b0850bd2a2119b30f7224d18a0e34c7bbf549ad7e3d36")
)



var TrustedCheckpoints = map[common.Hash]*TrustedCheckpoint{
	MainnetGenesisHash: MainnetTrustedCheckpoint,
	RopstenGenesisHash: RopstenTrustedCheckpoint,
	RinkebyGenesisHash: RinkebyTrustedCheckpoint,
	GoerliGenesisHash:  GoerliTrustedCheckpoint,
}



var CheckpointOracles = map[common.Hash]*CheckpointOracleConfig{
	MainnetGenesisHash: MainnetCheckpointOracle,
	RopstenGenesisHash: RopstenCheckpointOracle,
	RinkebyGenesisHash: RinkebyCheckpointOracle,
	GoerliGenesisHash:  GoerliCheckpointOracle,
}

var (
	
	MainnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(1),
		HomesteadBlock:      big.NewInt(1150000),
		DAOForkBlock:        big.NewInt(1920000),
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(2463000),
		EIP150Hash:          common.HexToHash("0x2086799aeebeae135c246c65021c82b4e15a2c451340993aacfd2751886514f0"),
		EIP155Block:         big.NewInt(2675000),
		EIP158Block:         big.NewInt(2675000),
		ByzantiumBlock:      big.NewInt(4370000),
		ConstantinopleBlock: big.NewInt(7280000),
		PetersburgBlock:     big.NewInt(7280000),
		IstanbulBlock:       big.NewInt(9069000),
		MuirGlacierBlock:    big.NewInt(9200000),
		Ethash:              new(EthashConfig),
	}

	
	MainnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 336,
		SectionHead:  common.HexToHash("0xd42b78902b6527a80337bf1bc372a3ccc3db97e9cc7cf421ca047ae9076c716b"),
		CHTRoot:      common.HexToHash("0xd97f3b30f7e0cb958e4c67c53ec27745e5a165e33e56821b86523dfee62b783a"),
		BloomRoot:    common.HexToHash("0xf3cbfd070fababfe2adc9b23fc02c731f6ca2cce6646b3ede4ef2db06092ccce"),
	}

	
	MainnetCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0x9a9070028361F7AAbeB3f2F2Dc07F82C4a98A02a"),
		Signers: []common.Address{
			common.HexToAddress("0x1b2C260efc720BE89101890E4Db589b44E950527"), 
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), 
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), 
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), 
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), 
		},
		Threshold: 2,
	}

	
	RopstenChainConfig = &ChainConfig{
		ChainID:             big.NewInt(3),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		EIP155Block:         big.NewInt(10),
		EIP158Block:         big.NewInt(10),
		ByzantiumBlock:      big.NewInt(1700000),
		ConstantinopleBlock: big.NewInt(4230000),
		PetersburgBlock:     big.NewInt(4939394),
		IstanbulBlock:       big.NewInt(6485846),
		MuirGlacierBlock:    big.NewInt(7117117),
		Ethash:              new(EthashConfig),
	}

	
	RopstenTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 269,
		SectionHead:  common.HexToHash("0x290a9eb65e65c64601d1b05522533ed502098a246736b348502a170818a33d64"),
		CHTRoot:      common.HexToHash("0x530ebac02264227277d0a16b0819ef96a2011a6e1e66523ebff8040f4a3437ca"),
		BloomRoot:    common.HexToHash("0x480cd5b3198a0767022902130546854a2e8867cce573c1cf0ce54e67a7bf5efb"),
	}

	
	RopstenCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0xEF79475013f154E6A65b54cB2742867791bf0B84"),
		Signers: []common.Address{
			common.HexToAddress("0x32162F3581E88a5f62e8A61892B42C46E2c18f7b"), 
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), 
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), 
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), 
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), 
		},
		Threshold: 2,
	}

	
	RinkebyChainConfig = &ChainConfig{
		ChainID:             big.NewInt(4),
		HomesteadBlock:      big.NewInt(1),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(2),
		EIP150Hash:          common.HexToHash("0x9b095b36c15eaf13044373aef8ee0bd3a382a5abb92e402afa44b8249c3a90e9"),
		EIP155Block:         big.NewInt(3),
		EIP158Block:         big.NewInt(3),
		ByzantiumBlock:      big.NewInt(1035301),
		ConstantinopleBlock: big.NewInt(3660663),
		PetersburgBlock:     big.NewInt(4321234),
		IstanbulBlock:       big.NewInt(5435345),
		MuirGlacierBlock:    nil,
		Clique: &CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	
	RinkebyTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 223,
		SectionHead:  common.HexToHash("0x03ca0d5e3a931c77cd7a97bbaa2d9e4edc4549c621dc1d223a29f10c86a4a16a"),
		CHTRoot:      common.HexToHash("0x6573dbdd91b2958b446bd04d67c23e5f14b2510ac96e8df1b6a894dc49e37c6c"),
		BloomRoot:    common.HexToHash("0x28a35042a4e88efbac55fe566faf7fce000dc436f17fd4cb4b081c9cd793e1a7"),
	}

	
	RinkebyCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0xebe8eFA441B9302A0d7eaECc277c09d20D684540"),
		Signers: []common.Address{
			common.HexToAddress("0xd9c9cd5f6779558b6e0ed4e6acf6b1947e7fa1f3"), 
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), 
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), 
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), 
		},
		Threshold: 2,
	}

	
	GoerliChainConfig = &ChainConfig{
		ChainID:             big.NewInt(5),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(1561651),
		MuirGlacierBlock:    nil,
		Clique: &CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	
	GoerliTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 107,
		SectionHead:  common.HexToHash("0xff3ae39199fa191894de419e7f673c8627aa8cc7af924b90f36635b6add375f2"),
		CHTRoot:      common.HexToHash("0x27d59d60c652425b6b593a882f55a4ff57f24e470a810a6e3c8ba71833a20220"),
		BloomRoot:    common.HexToHash("0x3c14066d8bb3733780c06b8165768dbb9dd23b75f56012fe5f2fb3c2fb70cadb"),
	}

	
	GoerliCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0x18CA0E045F0D772a851BC7e48357Bcaab0a0795D"),
		Signers: []common.Address{
			common.HexToAddress("0x4769bcaD07e3b938B7f43EB7D278Bc7Cb9efFb38"), 
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), 
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), 
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), 
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), 
		},
		Threshold: 2,
	}

	
	YoloV1ChainConfig = &ChainConfig{
		ChainID:             big.NewInt(133519467574833),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    nil,
		YoloV1Block:         big.NewInt(0),
		Clique: &CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	
	
	
	
	
	AllEthashProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil, nil, new(EthashConfig), nil}

	
	
	
	
	
	AllCliqueProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil, nil, nil, &CliqueConfig{Period: 0, Epoch: 30000}}

	TestChainConfig = &ChainConfig{big.NewInt(1), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil, nil, new(EthashConfig), nil}
	TestRules       = TestChainConfig.Rules(new(big.Int))
)





type TrustedCheckpoint struct {
	SectionIndex uint64      `json:"sectionIndex"`
	SectionHead  common.Hash `json:"sectionHead"`
	CHTRoot      common.Hash `json:"chtRoot"`
	BloomRoot    common.Hash `json:"bloomRoot"`
}


func (c *TrustedCheckpoint) HashEqual(hash common.Hash) bool {
	if c.Empty() {
		return hash == common.Hash{}
	}
	return c.Hash() == hash
}


func (c *TrustedCheckpoint) Hash() common.Hash {
	buf := make([]byte, 8+3*common.HashLength)
	binary.BigEndian.PutUint64(buf, c.SectionIndex)
	copy(buf[8:], c.SectionHead.Bytes())
	copy(buf[8+common.HashLength:], c.CHTRoot.Bytes())
	copy(buf[8+2*common.HashLength:], c.BloomRoot.Bytes())
	return crypto.Keccak256Hash(buf)
}


func (c *TrustedCheckpoint) Empty() bool {
	return c.SectionHead == (common.Hash{}) || c.CHTRoot == (common.Hash{}) || c.BloomRoot == (common.Hash{})
}



type CheckpointOracleConfig struct {
	Address   common.Address   `json:"address"`
	Signers   []common.Address `json:"signers"`
	Threshold uint64           `json:"threshold"`
}






type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` 

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` 

	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"` 

	
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` 
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  

	EIP155Block *big.Int `json:"eip155Block,omitempty"` 
	EIP158Block *big.Int `json:"eip158Block,omitempty"` 

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` 
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`     
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`       
	MuirGlacierBlock    *big.Int `json:"muirGlacierBlock,omitempty"`    

	YoloV1Block *big.Int `json:"yoloV1Block,omitempty"` 
	EWASMBlock  *big.Int `json:"ewasmBlock,omitempty"`  

	
	Ethash *EthashConfig `json:"ethash,omitempty"`
	Clique *CliqueConfig `json:"clique,omitempty"`
}


type EthashConfig struct{}


func (c *EthashConfig) String() string {
	return "ethash"
}


type CliqueConfig struct {
	Period uint64 `json:"period"` 
	Epoch  uint64 `json:"epoch"`  
}


func (c *CliqueConfig) String() string {
	return "clique"
}


func (c *ChainConfig) String() string {
	var engine interface{}
	switch {
	case c.Ethash != nil:
		engine = c.Ethash
	case c.Clique != nil:
		engine = c.Clique
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{ChainID: %v Homestead: %v DAO: %v DAOSupport: %v EIP150: %v EIP155: %v EIP158: %v Byzantium: %v Constantinople: %v Petersburg: %v Istanbul: %v, Muir Glacier: %v, YOLO v1: %v, Engine: %v}",
		c.ChainID,
		c.HomesteadBlock,
		c.DAOForkBlock,
		c.DAOForkSupport,
		c.EIP150Block,
		c.EIP155Block,
		c.EIP158Block,
		c.ByzantiumBlock,
		c.ConstantinopleBlock,
		c.PetersburgBlock,
		c.IstanbulBlock,
		c.MuirGlacierBlock,
		c.YoloV1Block,
		engine,
	)
}


func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(c.HomesteadBlock, num)
}


func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return isForked(c.DAOForkBlock, num)
}


func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(c.EIP150Block, num)
}


func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(c.EIP155Block, num)
}


func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(c.EIP158Block, num)
}


func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(c.ByzantiumBlock, num)
}


func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(c.ConstantinopleBlock, num)
}


func (c *ChainConfig) IsMuirGlacier(num *big.Int) bool {
	return isForked(c.MuirGlacierBlock, num)
}




func (c *ChainConfig) IsPetersburg(num *big.Int) bool {
	return isForked(c.PetersburgBlock, num) || c.PetersburgBlock == nil && isForked(c.ConstantinopleBlock, num)
}


func (c *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isForked(c.IstanbulBlock, num)
}


func (c *ChainConfig) IsYoloV1(num *big.Int) bool {
	return isForked(c.YoloV1Block, num)
}


func (c *ChainConfig) IsEWASM(num *big.Int) bool {
	return isForked(c.EWASMBlock, num)
}



func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	
	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}



func (c *ChainConfig) CheckConfigForkOrder() error {
	type fork struct {
		name     string
		block    *big.Int
		optional bool 
	}
	var lastFork fork
	for _, cur := range []fork{
		{name: "homesteadBlock", block: c.HomesteadBlock},
		{name: "daoForkBlock", block: c.DAOForkBlock, optional: true},
		{name: "eip150Block", block: c.EIP150Block},
		{name: "eip155Block", block: c.EIP155Block},
		{name: "eip158Block", block: c.EIP158Block},
		{name: "byzantiumBlock", block: c.ByzantiumBlock},
		{name: "constantinopleBlock", block: c.ConstantinopleBlock},
		{name: "petersburgBlock", block: c.PetersburgBlock},
		{name: "istanbulBlock", block: c.IstanbulBlock},
		{name: "muirGlacierBlock", block: c.MuirGlacierBlock, optional: true},
		{name: "yoloV1Block", block: c.YoloV1Block},
	} {
		if lastFork.name != "" {
			
			if lastFork.block == nil && cur.block != nil {
				return fmt.Errorf("unsupported fork ordering: %v not enabled, but %v enabled at %v",
					lastFork.name, cur.name, cur.block)
			}
			if lastFork.block != nil && cur.block != nil {
				if lastFork.block.Cmp(cur.block) > 0 {
					return fmt.Errorf("unsupported fork ordering: %v enabled at %v, but %v enabled at %v",
						lastFork.name, lastFork.block, cur.name, cur.block)
				}
			}
		}
		
		if !cur.optional || cur.block != nil {
			lastFork = cur
		}
	}
	return nil
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(c.DAOForkBlock, newcfg.DAOForkBlock, head) {
		return newCompatError("DAO fork block", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if c.IsDAOFork(head) && c.DAOForkSupport != newcfg.DAOForkSupport {
		return newCompatError("DAO fork support flag", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if isForkIncompatible(c.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(c.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(c.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
	}
	if c.IsEIP158(head) && !configNumEqual(c.ChainID, newcfg.ChainID) {
		return newCompatError("EIP158 chain ID", c.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	if isForkIncompatible(c.PetersburgBlock, newcfg.PetersburgBlock, head) {
		
		
		if isForkIncompatible(c.ConstantinopleBlock, newcfg.PetersburgBlock, head) {
			return newCompatError("Petersburg fork block", c.PetersburgBlock, newcfg.PetersburgBlock)
		}
	}
	if isForkIncompatible(c.IstanbulBlock, newcfg.IstanbulBlock, head) {
		return newCompatError("Istanbul fork block", c.IstanbulBlock, newcfg.IstanbulBlock)
	}
	if isForkIncompatible(c.MuirGlacierBlock, newcfg.MuirGlacierBlock, head) {
		return newCompatError("Muir Glacier fork block", c.MuirGlacierBlock, newcfg.MuirGlacierBlock)
	}
	if isForkIncompatible(c.YoloV1Block, newcfg.YoloV1Block, head) {
		return newCompatError("YOLOv1 fork block", c.YoloV1Block, newcfg.YoloV1Block)
	}
	if isForkIncompatible(c.EWASMBlock, newcfg.EWASMBlock, head) {
		return newCompatError("ewasm fork block", c.EWASMBlock, newcfg.EWASMBlock)
	}
	return nil
}



func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}


func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}



type ConfigCompatError struct {
	What string
	
	StoredConfig, NewConfig *big.Int
	
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}






type Rules struct {
	ChainID                                                 *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
	IsYoloV1                                                bool
}


func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID:          new(big.Int).Set(chainID),
		IsHomestead:      c.IsHomestead(num),
		IsEIP150:         c.IsEIP150(num),
		IsEIP155:         c.IsEIP155(num),
		IsEIP158:         c.IsEIP158(num),
		IsByzantium:      c.IsByzantium(num),
		IsConstantinople: c.IsConstantinople(num),
		IsPetersburg:     c.IsPetersburg(num),
		IsIstanbul:       c.IsIstanbul(num),
		IsYoloV1:         c.IsYoloV1(num),
	}
}

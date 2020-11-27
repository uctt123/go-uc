
















package checkpointoracle



import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/checkpointoracle/contract"
	"github.com/ethereum/go-ethereum/core/types"
)


type CheckpointOracle struct {
	address  common.Address
	contract *contract.CheckpointOracle
}


func NewCheckpointOracle(contractAddr common.Address, backend bind.ContractBackend) (*CheckpointOracle, error) {
	c, err := contract.NewCheckpointOracle(contractAddr, backend)
	if err != nil {
		return nil, err
	}
	return &CheckpointOracle{address: contractAddr, contract: c}, nil
}


func (oracle *CheckpointOracle) ContractAddr() common.Address {
	return oracle.address
}


func (oracle *CheckpointOracle) Contract() *contract.CheckpointOracle {
	return oracle.contract
}



func (oracle *CheckpointOracle) LookupCheckpointEvents(blockLogs [][]*types.Log, section uint64, hash common.Hash) []*contract.CheckpointOracleNewCheckpointVote {
	var votes []*contract.CheckpointOracleNewCheckpointVote

	for _, logs := range blockLogs {
		for _, log := range logs {
			event, err := oracle.contract.ParseNewCheckpointVote(*log)
			if err != nil {
				continue
			}
			if event.Index == section && common.Hash(event.CheckpointHash) == hash {
				votes = append(votes, event)
			}
		}
	}
	return votes
}






func (oracle *CheckpointOracle) RegisterCheckpoint(opts *bind.TransactOpts, index uint64, hash []byte, rnum *big.Int, rhash [32]byte, sigs [][]byte) (*types.Transaction, error) {
	var (
		r [][32]byte
		s [][32]byte
		v []uint8
	)
	for i := 0; i < len(sigs); i++ {
		if len(sigs[i]) != 65 {
			return nil, errors.New("invalid signature")
		}
		r = append(r, common.BytesToHash(sigs[i][:32]))
		s = append(s, common.BytesToHash(sigs[i][32:64]))
		v = append(v, sigs[i][64])
	}
	return oracle.contract.SetCheckpoint(opts, rnum, rhash, common.BytesToHash(hash), index, v, r, s)
}

















package main

import (
	"strconv"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/external"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/checkpointoracle"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"gopkg.in/urfave/cli.v1"
)


func newClient(ctx *cli.Context) *ethclient.Client {
	client, err := ethclient.Dial(ctx.GlobalString(nodeURLFlag.Name))
	if err != nil {
		utils.Fatalf("Failed to connect to Ethereum node: %v", err)
	}
	return client
}


func newRPCClient(url string) *rpc.Client {
	client, err := rpc.Dial(url)
	if err != nil {
		utils.Fatalf("Failed to connect to Ethereum node: %v", err)
	}
	return client
}



func getContractAddr(client *rpc.Client) common.Address {
	var addr string
	if err := client.Call(&addr, "les_getCheckpointContractAddress"); err != nil {
		utils.Fatalf("Failed to fetch checkpoint oracle address: %v", err)
	}
	return common.HexToAddress(addr)
}



func getCheckpoint(ctx *cli.Context, client *rpc.Client) *params.TrustedCheckpoint {
	var checkpoint *params.TrustedCheckpoint

	if ctx.GlobalIsSet(indexFlag.Name) {
		var result [3]string
		index := uint64(ctx.GlobalInt64(indexFlag.Name))
		if err := client.Call(&result, "les_getCheckpoint", index); err != nil {
			utils.Fatalf("Failed to get local checkpoint %v, please ensure the les API is exposed", err)
		}
		checkpoint = &params.TrustedCheckpoint{
			SectionIndex: index,
			SectionHead:  common.HexToHash(result[0]),
			CHTRoot:      common.HexToHash(result[1]),
			BloomRoot:    common.HexToHash(result[2]),
		}
	} else {
		var result [4]string
		err := client.Call(&result, "les_latestCheckpoint")
		if err != nil {
			utils.Fatalf("Failed to get local checkpoint %v, please ensure the les API is exposed", err)
		}
		index, err := strconv.ParseUint(result[0], 0, 64)
		if err != nil {
			utils.Fatalf("Failed to parse checkpoint index %v", err)
		}
		checkpoint = &params.TrustedCheckpoint{
			SectionIndex: index,
			SectionHead:  common.HexToHash(result[1]),
			CHTRoot:      common.HexToHash(result[2]),
			BloomRoot:    common.HexToHash(result[3]),
		}
	}
	return checkpoint
}



func newContract(client *rpc.Client) (common.Address, *checkpointoracle.CheckpointOracle) {
	addr := getContractAddr(client)
	if addr == (common.Address{}) {
		utils.Fatalf("No specified registrar contract address")
	}
	contract, err := checkpointoracle.NewCheckpointOracle(addr, ethclient.NewClient(client))
	if err != nil {
		utils.Fatalf("Failed to setup registrar contract %s: %v", addr, err)
	}
	return addr, contract
}


func newClefSigner(ctx *cli.Context) *bind.TransactOpts {
	clef, err := external.NewExternalSigner(ctx.String(clefURLFlag.Name))
	if err != nil {
		utils.Fatalf("Failed to create clef signer %v", err)
	}
	return bind.NewClefTransactor(clef, accounts.Account{Address: common.HexToAddress(ctx.String(signerFlag.Name))})
}

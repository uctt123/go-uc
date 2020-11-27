















package main

import (
	"fmt"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/urfave/cli.v1"
)

var commandStatus = cli.Command{
	Name:  "status",
	Usage: "Fetches the signers and checkpoint status of the oracle contract",
	Flags: []cli.Flag{
		nodeURLFlag,
	},
	Action: utils.MigrateFlags(status),
}


func status(ctx *cli.Context) error {
	
	addr, oracle := newContract(newRPCClient(ctx.GlobalString(nodeURLFlag.Name)))
	fmt.Printf("Oracle => %s\n", addr.Hex())
	fmt.Println()

	
	admins, err := oracle.Contract().GetAllAdmin(nil)
	if err != nil {
		return err
	}
	for i, admin := range admins {
		fmt.Printf("Admin %d => %s\n", i+1, admin.Hex())
	}
	fmt.Println()

	
	index, checkpoint, height, err := oracle.Contract().GetLatestCheckpoint(nil)
	if err != nil {
		return err
	}
	fmt.Printf("Checkpoint (published at #%d) %d => %s\n", height, index, common.Hash(checkpoint).Hex())

	return nil
}

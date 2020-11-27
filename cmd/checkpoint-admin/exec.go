















package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/contracts/checkpointoracle"
	"github.com/ethereum/go-ethereum/contracts/checkpointoracle/contract"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"gopkg.in/urfave/cli.v1"
)

var commandDeploy = cli.Command{
	Name:  "deploy",
	Usage: "Deploy a new checkpoint oracle contract",
	Flags: []cli.Flag{
		nodeURLFlag,
		clefURLFlag,
		signerFlag,
		signersFlag,
		thresholdFlag,
	},
	Action: utils.MigrateFlags(deploy),
}

var commandSign = cli.Command{
	Name:  "sign",
	Usage: "Sign the checkpoint with the specified key",
	Flags: []cli.Flag{
		nodeURLFlag,
		clefURLFlag,
		signerFlag,
		indexFlag,
		hashFlag,
		oracleFlag,
	},
	Action: utils.MigrateFlags(sign),
}

var commandPublish = cli.Command{
	Name:  "publish",
	Usage: "Publish a checkpoint into the oracle",
	Flags: []cli.Flag{
		nodeURLFlag,
		clefURLFlag,
		signerFlag,
		indexFlag,
		signaturesFlag,
	},
	Action: utils.MigrateFlags(publish),
}





func deploy(ctx *cli.Context) error {
	
	var addrs []common.Address
	for _, account := range strings.Split(ctx.String(signersFlag.Name), ",") {
		if trimmed := strings.TrimSpace(account); !common.IsHexAddress(trimmed) {
			utils.Fatalf("Invalid account in --signers: '%s'", trimmed)
		}
		addrs = append(addrs, common.HexToAddress(account))
	}
	
	needed := ctx.Int(thresholdFlag.Name)
	if needed == 0 || needed > len(addrs) {
		utils.Fatalf("Invalid signature threshold %d", needed)
	}
	
	fmt.Printf("Deploying new checkpoint oracle:\n\n")
	for i, addr := range addrs {
		fmt.Printf("Admin %d => %s\n", i+1, addr.Hex())
	}
	fmt.Printf("\nSignatures needed to publish: %d\n", needed)

	
	transactor, client := newClefSigner(ctx), newClient(ctx)

	
	fmt.Println("Sending deploy request to Clef...")
	oracle, tx, _, err := contract.DeployCheckpointOracle(transactor, client, addrs, big.NewInt(int64(params.CheckpointFrequency)),
		big.NewInt(int64(params.CheckpointProcessConfirmations)), big.NewInt(int64(needed)))
	if err != nil {
		utils.Fatalf("Failed to deploy checkpoint oracle %v", err)
	}
	log.Info("Deployed checkpoint oracle", "address", oracle, "tx", tx.Hash().Hex())

	return nil
}




func sign(ctx *cli.Context) error {
	var (
		offline bool 
		chash   common.Hash
		cindex  uint64
		address common.Address

		node   *rpc.Client
		oracle *checkpointoracle.CheckpointOracle
	)
	if !ctx.GlobalIsSet(nodeURLFlag.Name) {
		
		offline = true
		if !ctx.IsSet(hashFlag.Name) {
			utils.Fatalf("Please specify the checkpoint hash (--hash) to sign in offline mode")
		}
		chash = common.HexToHash(ctx.String(hashFlag.Name))

		if !ctx.IsSet(indexFlag.Name) {
			utils.Fatalf("Please specify checkpoint index (--index) to sign in offline mode")
		}
		cindex = ctx.Uint64(indexFlag.Name)

		if !ctx.IsSet(oracleFlag.Name) {
			utils.Fatalf("Please specify oracle address (--oracle) to sign in offline mode")
		}
		address = common.HexToAddress(ctx.String(oracleFlag.Name))
	} else {
		
		node = newRPCClient(ctx.GlobalString(nodeURLFlag.Name))

		checkpoint := getCheckpoint(ctx, node)
		chash, cindex, address = checkpoint.Hash(), checkpoint.SectionIndex, getContractAddr(node)

		
		reqCtx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelFn()

		head, err := ethclient.NewClient(node).HeaderByNumber(reqCtx, nil)
		if err != nil {
			return err
		}
		num := head.Number.Uint64()
		if num < ((cindex+1)*params.CheckpointFrequency + params.CheckpointProcessConfirmations) {
			utils.Fatalf("Invalid future checkpoint")
		}
		_, oracle = newContract(node)
		latest, _, h, err := oracle.Contract().GetLatestCheckpoint(nil)
		if err != nil {
			return err
		}
		if cindex < latest {
			utils.Fatalf("Checkpoint is too old")
		}
		if cindex == latest && (latest != 0 || h.Uint64() != 0) {
			utils.Fatalf("Stale checkpoint, latest registered %d, given %d", latest, cindex)
		}
	}
	var (
		signature string
		signer    string
	)
	
	isAdmin := func(addr common.Address) error {
		signers, err := oracle.Contract().GetAllAdmin(nil)
		if err != nil {
			return err
		}
		for _, s := range signers {
			if s == addr {
				return nil
			}
		}
		return fmt.Errorf("signer %v is not the admin", addr.Hex())
	}
	
	fmt.Printf("Oracle     => %s\n", address.Hex())
	fmt.Printf("Index %4d => %s\n", cindex, chash.Hex())

	
	signer = ctx.String(signerFlag.Name)

	if !offline {
		if err := isAdmin(common.HexToAddress(signer)); err != nil {
			return err
		}
	}
	clef := newRPCClient(ctx.String(clefURLFlag.Name))
	p := make(map[string]string)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, cindex)
	p["address"] = address.Hex()
	p["message"] = hexutil.Encode(append(buf, chash.Bytes()...))

	fmt.Println("Sending signing request to Clef...")
	if err := clef.Call(&signature, "account_signData", accounts.MimetypeDataWithValidator, signer, p); err != nil {
		utils.Fatalf("Failed to sign checkpoint, err %v", err)
	}
	fmt.Printf("Signer     => %s\n", signer)
	fmt.Printf("Signature  => %s\n", signature)
	return nil
}


func sighash(index uint64, oracle common.Address, hash common.Hash) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, index)

	data := append([]byte{0x19, 0x00}, append(oracle[:], append(buf, hash[:]...)...)...)
	return crypto.Keccak256(data)
}


func ecrecover(sighash []byte, sig []byte) common.Address {
	sig[64] -= 27
	defer func() { sig[64] += 27 }()

	signer, err := crypto.SigToPub(sighash, sig)
	if err != nil {
		utils.Fatalf("Failed to recover sender from signature %x: %v", sig, err)
	}
	return crypto.PubkeyToAddress(*signer)
}



func publish(ctx *cli.Context) error {
	
	
	status(ctx)

	
	var sigs [][]byte
	for _, sig := range strings.Split(ctx.String(signaturesFlag.Name), ",") {
		trimmed := strings.TrimPrefix(strings.TrimSpace(sig), "0x")
		if len(trimmed) != 130 {
			utils.Fatalf("Invalid signature in --signature: '%s'", trimmed)
		} else {
			sigs = append(sigs, common.Hex2Bytes(trimmed))
		}
	}
	
	var (
		client       = newRPCClient(ctx.GlobalString(nodeURLFlag.Name))
		addr, oracle = newContract(client)
		checkpoint   = getCheckpoint(ctx, client)
		sighash      = sighash(checkpoint.SectionIndex, addr, checkpoint.Hash())
	)
	for i := 0; i < len(sigs); i++ {
		for j := i + 1; j < len(sigs); j++ {
			signerA := ecrecover(sighash, sigs[i])
			signerB := ecrecover(sighash, sigs[j])
			if bytes.Compare(signerA.Bytes(), signerB.Bytes()) > 0 {
				sigs[i], sigs[j] = sigs[j], sigs[i]
			}
		}
	}
	
	reqCtx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()

	head, err := ethclient.NewClient(client).HeaderByNumber(reqCtx, nil)
	if err != nil {
		return err
	}
	num := head.Number.Uint64()
	recent, err := ethclient.NewClient(client).HeaderByNumber(reqCtx, big.NewInt(int64(num-128)))
	if err != nil {
		return err
	}
	
	fmt.Printf("Publishing %d => %s:\n\n", checkpoint.SectionIndex, checkpoint.Hash().Hex())
	for i, sig := range sigs {
		fmt.Printf("Signer %d => %s\n", i+1, ecrecover(sighash, sig).Hex())
	}
	fmt.Println()
	fmt.Printf("Sentry number => %d\nSentry hash   => %s\n", recent.Number, recent.Hash().Hex())

	
	fmt.Println("Sending publish request to Clef...")
	tx, err := oracle.RegisterCheckpoint(newClefSigner(ctx), checkpoint.SectionIndex, checkpoint.Hash().Bytes(), recent.Number, recent.Hash(), sigs)
	if err != nil {
		utils.Fatalf("Register contract failed %v", err)
	}
	log.Info("Successfully registered checkpoint", "tx", tx.Hash().Hex())
	return nil
}

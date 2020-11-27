















package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
)


func (w *wizard) deployWallet() {
	
	if w.conf.Genesis == nil {
		log.Error("No genesis block configured")
		return
	}
	if w.conf.ethstats == "" {
		log.Error("No ethstats server configured")
		return
	}
	
	server := w.selectServer()
	if server == "" {
		return
	}
	client := w.servers[server]

	
	infos, err := checkWallet(client, w.network)
	if err != nil {
		infos = &walletInfos{
			nodePort: 30303, rpcPort: 8545, webPort: 80, webHost: client.server,
		}
	}
	existed := err == nil

	infos.genesis, _ = json.MarshalIndent(w.conf.Genesis, "", "  ")
	infos.network = w.conf.Genesis.Config.ChainID.Int64()

	
	fmt.Println()
	fmt.Printf("Which port should the wallet listen on? (default = %d)\n", infos.webPort)
	infos.webPort = w.readDefaultInt(infos.webPort)

	
	if infos.webHost, err = w.ensureVirtualHost(client, infos.webPort, infos.webHost); err != nil {
		log.Error("Failed to decide on wallet host", "err", err)
		return
	}
	
	fmt.Println()
	if infos.datadir == "" {
		fmt.Printf("Where should data be stored on the remote machine?\n")
		infos.datadir = w.readString()
	} else {
		fmt.Printf("Where should data be stored on the remote machine? (default = %s)\n", infos.datadir)
		infos.datadir = w.readDefaultString(infos.datadir)
	}
	
	fmt.Println()
	fmt.Printf("Which TCP/UDP port should the backing node listen on? (default = %d)\n", infos.nodePort)
	infos.nodePort = w.readDefaultInt(infos.nodePort)

	fmt.Println()
	fmt.Printf("Which port should the backing RPC API listen on? (default = %d)\n", infos.rpcPort)
	infos.rpcPort = w.readDefaultInt(infos.rpcPort)

	
	fmt.Println()
	if infos.ethstats == "" {
		fmt.Printf("What should the wallet be called on the stats page?\n")
		infos.ethstats = w.readString() + ":" + w.conf.ethstats
	} else {
		fmt.Printf("What should the wallet be called on the stats page? (default = %s)\n", infos.ethstats)
		infos.ethstats = w.readDefaultString(infos.ethstats) + ":" + w.conf.ethstats
	}
	
	nocache := false
	if existed {
		fmt.Println()
		fmt.Printf("Should the wallet be built from scratch (y/n)? (default = no)\n")
		nocache = w.readDefaultYesNo(false)
	}
	if out, err := deployWallet(client, w.network, w.conf.bootnodes, infos, nocache); err != nil {
		log.Error("Failed to deploy wallet container", "err", err)
		if len(out) > 0 {
			fmt.Printf("%s\n", out)
		}
		return
	}
	
	log.Info("Waiting for node to finish booting")
	time.Sleep(3 * time.Second)

	w.networkStats()
}

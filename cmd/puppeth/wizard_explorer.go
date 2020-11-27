















package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
)


func (w *wizard) deployExplorer() {
	
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

	
	infos, err := checkExplorer(client, w.network)
	if err != nil {
		infos = &explorerInfos{
			node: &nodeInfos{port: 30303},
			port: 80,
			host: client.server,
		}
	}
	existed := err == nil

	infos.node.genesis, _ = json.MarshalIndent(w.conf.Genesis, "", "  ")
	infos.node.network = w.conf.Genesis.Config.ChainID.Int64()

	
	fmt.Println()
	fmt.Printf("Which port should the explorer listen on? (default = %d)\n", infos.port)
	infos.port = w.readDefaultInt(infos.port)

	
	if infos.host, err = w.ensureVirtualHost(client, infos.port, infos.host); err != nil {
		log.Error("Failed to decide on explorer host", "err", err)
		return
	}
	
	fmt.Println()
	if infos.node.datadir == "" {
		fmt.Printf("Where should node data be stored on the remote machine?\n")
		infos.node.datadir = w.readString()
	} else {
		fmt.Printf("Where should node data be stored on the remote machine? (default = %s)\n", infos.node.datadir)
		infos.node.datadir = w.readDefaultString(infos.node.datadir)
	}
	
	fmt.Println()
	if infos.dbdir == "" {
		fmt.Printf("Where should postgres data be stored on the remote machine?\n")
		infos.dbdir = w.readString()
	} else {
		fmt.Printf("Where should postgres data be stored on the remote machine? (default = %s)\n", infos.dbdir)
		infos.dbdir = w.readDefaultString(infos.dbdir)
	}
	
	fmt.Println()
	fmt.Printf("Which TCP/UDP port should the archive node listen on? (default = %d)\n", infos.node.port)
	infos.node.port = w.readDefaultInt(infos.node.port)

	
	fmt.Println()
	if infos.node.ethstats == "" {
		fmt.Printf("What should the explorer be called on the stats page?\n")
		infos.node.ethstats = w.readString() + ":" + w.conf.ethstats
	} else {
		fmt.Printf("What should the explorer be called on the stats page? (default = %s)\n", infos.node.ethstats)
		infos.node.ethstats = w.readDefaultString(infos.node.ethstats) + ":" + w.conf.ethstats
	}
	
	nocache := false
	if existed {
		fmt.Println()
		fmt.Printf("Should the explorer be built from scratch (y/n)? (default = no)\n")
		nocache = w.readDefaultYesNo(false)
	}
	if out, err := deployExplorer(client, w.network, w.conf.bootnodes, infos, nocache, w.conf.Genesis.Config.Clique != nil); err != nil {
		log.Error("Failed to deploy explorer container", "err", err)
		if len(out) > 0 {
			fmt.Printf("%s\n", out)
		}
		return
	}
	
	log.Info("Waiting for node to finish booting")
	time.Sleep(3 * time.Second)

	w.networkStats()
}

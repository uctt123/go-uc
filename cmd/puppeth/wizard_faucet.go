















package main

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/log"
)



func (w *wizard) deployFaucet() {
	
	server := w.selectServer()
	if server == "" {
		return
	}
	client := w.servers[server]

	
	infos, err := checkFaucet(client, w.network)
	if err != nil {
		infos = &faucetInfos{
			node:    &nodeInfos{port: 30303, peersTotal: 25},
			port:    80,
			host:    client.server,
			amount:  1,
			minutes: 1440,
			tiers:   3,
		}
	}
	existed := err == nil

	infos.node.genesis, _ = json.MarshalIndent(w.conf.Genesis, "", "  ")
	infos.node.network = w.conf.Genesis.Config.ChainID.Int64()

	
	fmt.Println()
	fmt.Printf("Which port should the faucet listen on? (default = %d)\n", infos.port)
	infos.port = w.readDefaultInt(infos.port)

	
	if infos.host, err = w.ensureVirtualHost(client, infos.port, infos.host); err != nil {
		log.Error("Failed to decide on faucet host", "err", err)
		return
	}
	
	fmt.Println()
	fmt.Printf("How many Ethers to release per request? (default = %d)\n", infos.amount)
	infos.amount = w.readDefaultInt(infos.amount)

	fmt.Println()
	fmt.Printf("How many minutes to enforce between requests? (default = %d)\n", infos.minutes)
	infos.minutes = w.readDefaultInt(infos.minutes)

	fmt.Println()
	fmt.Printf("How many funding tiers to feature (x2.5 amounts, x3 timeout)? (default = %d)\n", infos.tiers)
	infos.tiers = w.readDefaultInt(infos.tiers)
	if infos.tiers == 0 {
		log.Error("At least one funding tier must be set")
		return
	}
	
	if infos.captchaToken != "" {
		fmt.Println()
		fmt.Println("Reuse previous reCaptcha API authorization (y/n)? (default = yes)")
		if !w.readDefaultYesNo(true) {
			infos.captchaToken, infos.captchaSecret = "", ""
		}
	}
	if infos.captchaToken == "" {
		
		fmt.Println()
		fmt.Println("Enable reCaptcha protection against robots (y/n)? (default = no)")
		if !w.readDefaultYesNo(false) {
			log.Warn("Users will be able to requests funds via automated scripts")
		} else {
			
			fmt.Println()
			fmt.Printf("What is the reCaptcha site key to authenticate human users?\n")
			infos.captchaToken = w.readString()

			fmt.Println()
			fmt.Printf("What is the reCaptcha secret key to verify authentications? (won't be echoed)\n")
			infos.captchaSecret = w.readPassword()
		}
	}
	
	fmt.Println()
	if infos.node.datadir == "" {
		fmt.Printf("Where should data be stored on the remote machine?\n")
		infos.node.datadir = w.readString()
	} else {
		fmt.Printf("Where should data be stored on the remote machine? (default = %s)\n", infos.node.datadir)
		infos.node.datadir = w.readDefaultString(infos.node.datadir)
	}
	
	fmt.Println()
	fmt.Printf("Which TCP/UDP port should the light client listen on? (default = %d)\n", infos.node.port)
	infos.node.port = w.readDefaultInt(infos.node.port)

	
	fmt.Println()
	if infos.node.ethstats == "" {
		fmt.Printf("What should the node be called on the stats page?\n")
		infos.node.ethstats = w.readString() + ":" + w.conf.ethstats
	} else {
		fmt.Printf("What should the node be called on the stats page? (default = %s)\n", infos.node.ethstats)
		infos.node.ethstats = w.readDefaultString(infos.node.ethstats) + ":" + w.conf.ethstats
	}
	
	if infos.node.keyJSON != "" {
		if key, err := keystore.DecryptKey([]byte(infos.node.keyJSON), infos.node.keyPass); err != nil {
			infos.node.keyJSON, infos.node.keyPass = "", ""
		} else {
			fmt.Println()
			fmt.Printf("Reuse previous (%s) funding account (y/n)? (default = yes)\n", key.Address.Hex())
			if !w.readDefaultYesNo(true) {
				infos.node.keyJSON, infos.node.keyPass = "", ""
			}
		}
	}
	for i := 0; i < 3 && infos.node.keyJSON == ""; i++ {
		fmt.Println()
		fmt.Println("Please paste the faucet's funding account key JSON:")
		infos.node.keyJSON = w.readJSON()

		fmt.Println()
		fmt.Println("What's the unlock password for the account? (won't be echoed)")
		infos.node.keyPass = w.readPassword()

		if _, err := keystore.DecryptKey([]byte(infos.node.keyJSON), infos.node.keyPass); err != nil {
			log.Error("Failed to decrypt key with given password")
			infos.node.keyJSON = ""
			infos.node.keyPass = ""
		}
	}
	
	noauth := "n"
	if infos.noauth {
		noauth = "y"
	}
	fmt.Println()
	fmt.Printf("Permit non-authenticated funding requests (y/n)? (default = %v)\n", infos.noauth)
	infos.noauth = w.readDefaultString(noauth) != "n"

	
	nocache := false
	if existed {
		fmt.Println()
		fmt.Printf("Should the faucet be built from scratch (y/n)? (default = no)\n")
		nocache = w.readDefaultYesNo(false)
	}
	if out, err := deployFaucet(client, w.network, w.conf.bootnodes, infos, nocache); err != nil {
		log.Error("Failed to deploy faucet container", "err", err)
		if len(out) > 0 {
			fmt.Printf("%s\n", out)
		}
		return
	}
	
	w.networkStats()
}

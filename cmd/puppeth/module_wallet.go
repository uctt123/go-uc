















package main

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/log"
)


var walletDockerfile = `
FROM puppeth/wallet:latest

ADD genesis.json /genesis.json

RUN \
  echo 'node server.js &'                     > wallet.sh && \
	echo 'geth --cache 512 init /genesis.json' >> wallet.sh && \
	echo $'exec geth --networkid {{.NetworkID}} --port {{.NodePort}} --bootnodes {{.Bootnodes}} --ethstats \'{{.Ethstats}}\' --cache=512 --http --http.addr=0.0.0.0 --http.corsdomain "*" --http.vhosts "*"' >> wallet.sh

RUN \
	sed -i 's/PuppethNetworkID/{{.NetworkID}}/g' dist/js/etherwallet-master.js && \
	sed -i 's/PuppethNetwork/{{.Network}}/g'     dist/js/etherwallet-master.js && \
	sed -i 's/PuppethDenom/{{.Denom}}/g'         dist/js/etherwallet-master.js && \
	sed -i 's/PuppethHost/{{.Host}}/g'           dist/js/etherwallet-master.js && \
	sed -i 's/PuppethRPCPort/{{.RPCPort}}/g'     dist/js/etherwallet-master.js

ENTRYPOINT ["/bin/sh", "wallet.sh"]
`



var walletComposefile = `
version: '2'
services:
  wallet:
    build: .
    image: {{.Network}}/wallet
    container_name: {{.Network}}_wallet_1
    ports:
      - "{{.NodePort}}:{{.NodePort}}"
      - "{{.NodePort}}:{{.NodePort}}/udp"
      - "{{.RPCPort}}:8545"{{if not .VHost}}
      - "{{.WebPort}}:80"{{end}}
    volumes:
      - {{.Datadir}}:/root/.ethereum
    environment:
      - NODE_PORT={{.NodePort}}/tcp
      - STATS={{.Ethstats}}{{if .VHost}}
      - VIRTUAL_HOST={{.VHost}}
      - VIRTUAL_PORT=80{{end}}
    logging:
      driver: "json-file"
      options:
        max-size: "1m"
        max-file: "10"
    restart: always
`




func deployWallet(client *sshClient, network string, bootnodes []string, config *walletInfos, nocache bool) ([]byte, error) {
	
	workdir := fmt.Sprintf("%d", rand.Int63())
	files := make(map[string][]byte)

	dockerfile := new(bytes.Buffer)
	template.Must(template.New("").Parse(walletDockerfile)).Execute(dockerfile, map[string]interface{}{
		"Network":   strings.ToTitle(network),
		"Denom":     strings.ToUpper(network),
		"NetworkID": config.network,
		"NodePort":  config.nodePort,
		"RPCPort":   config.rpcPort,
		"Bootnodes": strings.Join(bootnodes, ","),
		"Ethstats":  config.ethstats,
		"Host":      client.address,
	})
	files[filepath.Join(workdir, "Dockerfile")] = dockerfile.Bytes()

	composefile := new(bytes.Buffer)
	template.Must(template.New("").Parse(walletComposefile)).Execute(composefile, map[string]interface{}{
		"Datadir":  config.datadir,
		"Network":  network,
		"NodePort": config.nodePort,
		"RPCPort":  config.rpcPort,
		"VHost":    config.webHost,
		"WebPort":  config.webPort,
		"Ethstats": config.ethstats[:strings.Index(config.ethstats, ":")],
	})
	files[filepath.Join(workdir, "docker-compose.yaml")] = composefile.Bytes()

	files[filepath.Join(workdir, "genesis.json")] = config.genesis

	
	if out, err := client.Upload(files); err != nil {
		return out, err
	}
	defer client.Run("rm -rf " + workdir)

	
	if nocache {
		return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s build --pull --no-cache && docker-compose -p %s up -d --force-recreate --timeout 60", workdir, network, network))
	}
	return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s up -d --build --force-recreate --timeout 60", workdir, network))
}



type walletInfos struct {
	genesis  []byte
	network  int64
	datadir  string
	ethstats string
	nodePort int
	rpcPort  int
	webHost  string
	webPort  int
}



func (info *walletInfos) Report() map[string]string {
	report := map[string]string{
		"Data directory":         info.datadir,
		"Ethstats username":      info.ethstats,
		"Node listener port ":    strconv.Itoa(info.nodePort),
		"RPC listener port ":     strconv.Itoa(info.rpcPort),
		"Website address ":       info.webHost,
		"Website listener port ": strconv.Itoa(info.webPort),
	}
	return report
}



func checkWallet(client *sshClient, network string) (*walletInfos, error) {
	
	infos, err := inspectContainer(client, fmt.Sprintf("%s_wallet_1", network))
	if err != nil {
		return nil, err
	}
	if !infos.running {
		return nil, ErrServiceOffline
	}
	
	webPort := infos.portmap["80/tcp"]
	if webPort == 0 {
		if proxy, _ := checkNginx(client, network); proxy != nil {
			webPort = proxy.port
		}
	}
	if webPort == 0 {
		return nil, ErrNotExposed
	}
	
	host := infos.envvars["VIRTUAL_HOST"]
	if host == "" {
		host = client.server
	}
	
	nodePort := infos.portmap[infos.envvars["NODE_PORT"]]
	if err = checkPort(client.server, nodePort); err != nil {
		log.Warn("Wallet devp2p port seems unreachable", "server", client.server, "port", nodePort, "err", err)
	}
	rpcPort := infos.portmap["8545/tcp"]
	if err = checkPort(client.server, rpcPort); err != nil {
		log.Warn("Wallet RPC port seems unreachable", "server", client.server, "port", rpcPort, "err", err)
	}
	
	stats := &walletInfos{
		datadir:  infos.volumes["/root/.ethereum"],
		nodePort: nodePort,
		rpcPort:  rpcPort,
		webHost:  host,
		webPort:  webPort,
		ethstats: infos.envvars["STATS"],
	}
	return stats, nil
}

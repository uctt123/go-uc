















package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)



var faucetDockerfile = `
FROM ethereum/client-go:alltools-latest

ADD genesis.json /genesis.json
ADD account.json /account.json
ADD account.pass /account.pass

EXPOSE 8080 30303 30303/udp

ENTRYPOINT [ \
	"faucet", "--genesis", "/genesis.json", "--network", "{{.NetworkID}}", "--bootnodes", "{{.Bootnodes}}", "--ethstats", "{{.Ethstats}}", "--ethport", "{{.EthPort}}",     \
	"--faucet.name", "{{.FaucetName}}", "--faucet.amount", "{{.FaucetAmount}}", "--faucet.minutes", "{{.FaucetMinutes}}", "--faucet.tiers", "{{.FaucetTiers}}",             \
	"--account.json", "/account.json", "--account.pass", "/account.pass"                                                                                                    \
	{{if .CaptchaToken}}, "--captcha.token", "{{.CaptchaToken}}", "--captcha.secret", "{{.CaptchaSecret}}"{{end}}{{if .NoAuth}}, "--noauth"{{end}}                          \
]`



var faucetComposefile = `
version: '2'
services:
  faucet:
    build: .
    image: {{.Network}}/faucet
    container_name: {{.Network}}_faucet_1
    ports:
      - "{{.EthPort}}:{{.EthPort}}"
      - "{{.EthPort}}:{{.EthPort}}/udp"{{if not .VHost}}
      - "{{.ApiPort}}:8080"{{end}}
    volumes:
      - {{.Datadir}}:/root/.faucet
    environment:
      - ETH_PORT={{.EthPort}}
      - ETH_NAME={{.EthName}}
      - FAUCET_AMOUNT={{.FaucetAmount}}
      - FAUCET_MINUTES={{.FaucetMinutes}}
      - FAUCET_TIERS={{.FaucetTiers}}
      - CAPTCHA_TOKEN={{.CaptchaToken}}
      - CAPTCHA_SECRET={{.CaptchaSecret}}
      - NO_AUTH={{.NoAuth}}{{if .VHost}}
      - VIRTUAL_HOST={{.VHost}}
      - VIRTUAL_PORT=8080{{end}}
    logging:
      driver: "json-file"
      options:
        max-size: "1m"
        max-file: "10"
    restart: always
`




func deployFaucet(client *sshClient, network string, bootnodes []string, config *faucetInfos, nocache bool) ([]byte, error) {
	
	workdir := fmt.Sprintf("%d", rand.Int63())
	files := make(map[string][]byte)

	dockerfile := new(bytes.Buffer)
	template.Must(template.New("").Parse(faucetDockerfile)).Execute(dockerfile, map[string]interface{}{
		"NetworkID":     config.node.network,
		"Bootnodes":     strings.Join(bootnodes, ","),
		"Ethstats":      config.node.ethstats,
		"EthPort":       config.node.port,
		"CaptchaToken":  config.captchaToken,
		"CaptchaSecret": config.captchaSecret,
		"FaucetName":    strings.Title(network),
		"FaucetAmount":  config.amount,
		"FaucetMinutes": config.minutes,
		"FaucetTiers":   config.tiers,
		"NoAuth":        config.noauth,
	})
	files[filepath.Join(workdir, "Dockerfile")] = dockerfile.Bytes()

	composefile := new(bytes.Buffer)
	template.Must(template.New("").Parse(faucetComposefile)).Execute(composefile, map[string]interface{}{
		"Network":       network,
		"Datadir":       config.node.datadir,
		"VHost":         config.host,
		"ApiPort":       config.port,
		"EthPort":       config.node.port,
		"EthName":       config.node.ethstats[:strings.Index(config.node.ethstats, ":")],
		"CaptchaToken":  config.captchaToken,
		"CaptchaSecret": config.captchaSecret,
		"FaucetAmount":  config.amount,
		"FaucetMinutes": config.minutes,
		"FaucetTiers":   config.tiers,
		"NoAuth":        config.noauth,
	})
	files[filepath.Join(workdir, "docker-compose.yaml")] = composefile.Bytes()

	files[filepath.Join(workdir, "genesis.json")] = config.node.genesis
	files[filepath.Join(workdir, "account.json")] = []byte(config.node.keyJSON)
	files[filepath.Join(workdir, "account.pass")] = []byte(config.node.keyPass)

	
	if out, err := client.Upload(files); err != nil {
		return out, err
	}
	defer client.Run("rm -rf " + workdir)

	
	if nocache {
		return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s build --pull --no-cache && docker-compose -p %s up -d --force-recreate --timeout 60", workdir, network, network))
	}
	return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s up -d --build --force-recreate --timeout 60", workdir, network))
}



type faucetInfos struct {
	node          *nodeInfos
	host          string
	port          int
	amount        int
	minutes       int
	tiers         int
	noauth        bool
	captchaToken  string
	captchaSecret string
}



func (info *faucetInfos) Report() map[string]string {
	report := map[string]string{
		"Website address":              info.host,
		"Website listener port":        strconv.Itoa(info.port),
		"Ethereum listener port":       strconv.Itoa(info.node.port),
		"Funding amount (base tier)":   fmt.Sprintf("%d Ethers", info.amount),
		"Funding cooldown (base tier)": fmt.Sprintf("%d mins", info.minutes),
		"Funding tiers":                strconv.Itoa(info.tiers),
		"Captha protection":            fmt.Sprintf("%v", info.captchaToken != ""),
		"Ethstats username":            info.node.ethstats,
	}
	if info.noauth {
		report["Debug mode (no auth)"] = "enabled"
	}
	if info.node.keyJSON != "" {
		var key struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal([]byte(info.node.keyJSON), &key); err == nil {
			report["Funding account"] = common.HexToAddress(key.Address).Hex()
		} else {
			log.Error("Failed to retrieve signer address", "err", err)
		}
	}
	return report
}



func checkFaucet(client *sshClient, network string) (*faucetInfos, error) {
	
	infos, err := inspectContainer(client, fmt.Sprintf("%s_faucet_1", network))
	if err != nil {
		return nil, err
	}
	if !infos.running {
		return nil, ErrServiceOffline
	}
	
	port := infos.portmap["8080/tcp"]
	if port == 0 {
		if proxy, _ := checkNginx(client, network); proxy != nil {
			port = proxy.port
		}
	}
	if port == 0 {
		return nil, ErrNotExposed
	}
	
	host := infos.envvars["VIRTUAL_HOST"]
	if host == "" {
		host = client.server
	}
	amount, _ := strconv.Atoi(infos.envvars["FAUCET_AMOUNT"])
	minutes, _ := strconv.Atoi(infos.envvars["FAUCET_MINUTES"])
	tiers, _ := strconv.Atoi(infos.envvars["FAUCET_TIERS"])

	
	var out []byte
	keyJSON, keyPass := "", ""
	if out, err = client.Run(fmt.Sprintf("docker exec %s_faucet_1 cat /account.json", network)); err == nil {
		keyJSON = string(bytes.TrimSpace(out))
	}
	if out, err = client.Run(fmt.Sprintf("docker exec %s_faucet_1 cat /account.pass", network)); err == nil {
		keyPass = string(bytes.TrimSpace(out))
	}
	
	if err = checkPort(host, port); err != nil {
		log.Warn("Faucet service seems unreachable", "server", host, "port", port, "err", err)
	}
	
	return &faucetInfos{
		node: &nodeInfos{
			datadir:  infos.volumes["/root/.faucet"],
			port:     infos.portmap[infos.envvars["ETH_PORT"]+"/tcp"],
			ethstats: infos.envvars["ETH_NAME"],
			keyJSON:  keyJSON,
			keyPass:  keyPass,
		},
		host:          host,
		port:          port,
		amount:        amount,
		minutes:       minutes,
		tiers:         tiers,
		captchaToken:  infos.envvars["CAPTCHA_TOKEN"],
		captchaSecret: infos.envvars["CAPTCHA_SECRET"],
		noauth:        infos.envvars["NO_AUTH"] == "true",
	}, nil
}

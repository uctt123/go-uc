
















package main

import (
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"gopkg.in/urfave/cli.v1"
)


func main() {
	app := cli.NewApp()
	app.Name = "puppeth"
	app.Usage = "assemble and maintain private Ethereum networks"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "network",
			Usage: "name of the network to administer (no spaces or hyphens, please)",
		},
		cli.IntFlag{
			Name:  "loglevel",
			Value: 3,
			Usage: "log level to emit to the screen",
		},
	}
	app.Before = func(c *cli.Context) error {
		
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(c.Int("loglevel")), log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
		rand.Seed(time.Now().UnixNano())

		return nil
	}
	app.Action = runWizard
	app.Run(os.Args)
}


func runWizard(c *cli.Context) error {
	network := c.String("network")
	if strings.Contains(network, " ") || strings.Contains(network, "-") || strings.ToLower(network) != network {
		log.Crit("No spaces, hyphens or capital letters allowed in network name")
	}
	makeWizard(c.String("network")).run()
	return nil
}

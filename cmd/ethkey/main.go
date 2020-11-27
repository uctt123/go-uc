















package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/internal/flags"
	"gopkg.in/urfave/cli.v1"
)

const (
	defaultKeyfileName = "keyfile.json"
)


var gitCommit = ""
var gitDate = ""

var app *cli.App

func init() {
	app = flags.NewApp(gitCommit, gitDate, "an Ethereum key manager")
	app.Commands = []cli.Command{
		commandGenerate,
		commandInspect,
		commandChangePassphrase,
		commandSignMessage,
		commandVerifyMessage,
	}
	cli.CommandHelpTemplate = flags.OriginCommandHelpTemplate
}


var (
	passphraseFlag = cli.StringFlag{
		Name:  "passwordfile",
		Usage: "the file that contains the password for the keyfile",
	}
	jsonFlag = cli.BoolFlag{
		Name:  "json",
		Usage: "output JSON instead of human-readable format",
	}
)

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

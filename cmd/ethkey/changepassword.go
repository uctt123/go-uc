















package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"gopkg.in/urfave/cli.v1"
)

var newPassphraseFlag = cli.StringFlag{
	Name:  "newpasswordfile",
	Usage: "the file that contains the new password for the keyfile",
}

var commandChangePassphrase = cli.Command{
	Name:      "changepassword",
	Usage:     "change the password on a keyfile",
	ArgsUsage: "<keyfile>",
	Description: `
Change the password of a keyfile.`,
	Flags: []cli.Flag{
		passphraseFlag,
		newPassphraseFlag,
	},
	Action: func(ctx *cli.Context) error {
		keyfilepath := ctx.Args().First()

		
		keyjson, err := ioutil.ReadFile(keyfilepath)
		if err != nil {
			utils.Fatalf("Failed to read the keyfile at '%s': %v", keyfilepath, err)
		}

		
		passphrase := getPassphrase(ctx, false)
		key, err := keystore.DecryptKey(keyjson, passphrase)
		if err != nil {
			utils.Fatalf("Error decrypting key: %v", err)
		}

		
		fmt.Println("Please provide a new password")
		var newPhrase string
		if passFile := ctx.String(newPassphraseFlag.Name); passFile != "" {
			content, err := ioutil.ReadFile(passFile)
			if err != nil {
				utils.Fatalf("Failed to read new password file '%s': %v", passFile, err)
			}
			newPhrase = strings.TrimRight(string(content), "\r\n")
		} else {
			newPhrase = utils.GetPassPhrase("", true)
		}

		
		newJson, err := keystore.EncryptKey(key, newPhrase, keystore.StandardScryptN, keystore.StandardScryptP)
		if err != nil {
			utils.Fatalf("Error encrypting with new password: %v", err)
		}

		
		if err := ioutil.WriteFile(keyfilepath, newJson, 0600); err != nil {
			utils.Fatalf("Error writing new keyfile to disk: %v", err)
		}

		
		
		return nil
	},
}

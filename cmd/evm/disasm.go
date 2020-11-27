















package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ethereum/go-ethereum/core/asm"
	cli "gopkg.in/urfave/cli.v1"
)

var disasmCommand = cli.Command{
	Action:    disasmCmd,
	Name:      "disasm",
	Usage:     "disassembles evm binary",
	ArgsUsage: "<file>",
}

func disasmCmd(ctx *cli.Context) error {
	var in string
	switch {
	case len(ctx.Args().First()) > 0:
		fn := ctx.Args().First()
		input, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		in = string(input)
	case ctx.GlobalIsSet(InputFlag.Name):
		in = ctx.GlobalString(InputFlag.Name)
	default:
		return errors.New("Missing filename or --input value")
	}

	code := strings.TrimSpace(in)
	fmt.Printf("%v\n", code)
	return asm.PrintDisassembled(code)
}

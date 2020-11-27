















package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/urfave/cli.v1"
)

var (
	
	gitCommit string
	gitDate   string
	app       = &cli.App{
		Name:        filepath.Base(os.Args[0]),
		Usage:       "go-ethereum devp2p tool",
		Version:     params.VersionWithCommit(gitCommit, gitDate),
		Writer:      os.Stdout,
		HideVersion: true,
	}
)

func init() {
	
	app.Flags = append(app.Flags, debug.Flags...)
	app.Before = func(ctx *cli.Context) error {
		return debug.Setup(ctx)
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		return nil
	}
	app.CommandNotFound = func(ctx *cli.Context, cmd string) {
		fmt.Fprintf(os.Stderr, "No such command: %s\n", cmd)
		os.Exit(1)
	}
	
	app.Commands = []cli.Command{
		enrdumpCommand,
		keyCommand,
		discv4Command,
		discv5Command,
		dnsCommand,
		nodesetCommand,
		rlpxCommand,
	}
}

func main() {
	exit(app.Run(os.Args))
}


func commandHasFlag(ctx *cli.Context, flag cli.Flag) bool {
	flags := ctx.FlagNames()
	sort.Strings(flags)
	i := sort.SearchStrings(flags, flag.GetName())
	return i != len(flags) && flags[i] == flag.GetName()
}


func getNodeArg(ctx *cli.Context) *enode.Node {
	if ctx.NArg() < 1 {
		exit("missing node as command-line argument")
	}
	n, err := parseNode(ctx.Args()[0])
	if err != nil {
		exit(err)
	}
	return n
}

func exit(err interface{}) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

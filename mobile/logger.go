















package geth

import (
	"os"

	"github.com/ethereum/go-ethereum/log"
)


func SetVerbosity(level int) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(level), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

















package discover

import (
	"crypto/ecdsa"
	"net"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)


type UDPConn interface {
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
}


type Config struct {
	
	PrivateKey *ecdsa.PrivateKey

	
	NetRestrict  *netutil.Netlist   
	Bootnodes    []*enode.Node      
	Unhandled    chan<- ReadPacket  
	Log          log.Logger         
	ValidSchemes enr.IdentityScheme 
	Clock        mclock.Clock
}

func (cfg Config) withDefaults() Config {
	if cfg.Log == nil {
		cfg.Log = log.Root()
	}
	if cfg.ValidSchemes == nil {
		cfg.ValidSchemes = enode.ValidSchemes
	}
	if cfg.Clock == nil {
		cfg.Clock = mclock.System{}
	}
	return cfg
}


func ListenUDP(c UDPConn, ln *enode.LocalNode, cfg Config) (*UDPv4, error) {
	return ListenV4(c, ln, cfg)
}



type ReadPacket struct {
	Data []byte
	Addr *net.UDPAddr
}

func min(x, y int) int {
	if x > y {
		return y
	}
	return x
}

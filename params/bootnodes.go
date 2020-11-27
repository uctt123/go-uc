















package params

import "github.com/ethereum/go-ethereum/common"



var MainnetBootnodes = []string{
	
	"enode:
	"enode:
	"enode:
	"enode:
	"enode:
	"enode:
	"enode:
	"enode:
}



var RopstenBootnodes = []string{
	"enode:
	"enode:
	"enode:
	"enode:
}



var RinkebyBootnodes = []string{
	"enode:
	"enode:
	"enode:
}



var GoerliBootnodes = []string{
	
	"enode:
	"enode:
	"enode:
	"enode:

	
	"enode:

	
	"enode:
	"enode:
	"enode:
}



var YoloV1Bootnodes = []string{
	"enode:
}

const dnsPrefix = "enrtree:




func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	var net string
	switch genesis {
	case MainnetGenesisHash:
		net = "mainnet"
	case RopstenGenesisHash:
		net = "ropsten"
	case RinkebyGenesisHash:
		net = "rinkeby"
	case GoerliGenesisHash:
		net = "goerli"
	default:
		return ""
	}
	return dnsPrefix + protocol + "." + net + ".ethdisco.net"
}

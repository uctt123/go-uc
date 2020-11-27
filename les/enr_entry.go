















package les

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)


type lesEntry struct {
	
	Rest []rlp.RawValue `rlp:"tail"`
}


func (e lesEntry) ENRKey() string {
	return "les"
}


func (eth *LightEthereum) setupDiscovery(cfg *p2p.Config) (enode.Iterator, error) {
	if /*cfg.NoDiscovery || */ len(eth.config.DiscoveryURLs) == 0 {
		return nil, nil
	}
	client := dnsdisc.NewClient(dnsdisc.Config{})
	return client.NewIterator(eth.config.DiscoveryURLs...)
}

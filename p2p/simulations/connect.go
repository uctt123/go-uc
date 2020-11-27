















package simulations

import (
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

var (
	ErrNodeNotFound = errors.New("node not found")
)





func (net *Network) ConnectToLastNode(id enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	ids := net.getUpNodeIDs()
	l := len(ids)
	if l < 2 {
		return nil
	}
	last := ids[l-1]
	if last == id {
		last = ids[l-2]
	}
	return net.connectNotConnected(last, id)
}



func (net *Network) ConnectToRandomNode(id enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	selected := net.getRandomUpNode(id)
	if selected == nil {
		return ErrNodeNotFound
	}
	return net.connectNotConnected(selected.ID(), id)
}




func (net *Network) ConnectNodesFull(ids []enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	if ids == nil {
		ids = net.getUpNodeIDs()
	}
	for i, lid := range ids {
		for _, rid := range ids[i+1:] {
			if err = net.connectNotConnected(lid, rid); err != nil {
				return err
			}
		}
	}
	return nil
}



func (net *Network) ConnectNodesChain(ids []enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	return net.connectNodesChain(ids)
}

func (net *Network) connectNodesChain(ids []enode.ID) (err error) {
	if ids == nil {
		ids = net.getUpNodeIDs()
	}
	l := len(ids)
	for i := 0; i < l-1; i++ {
		if err := net.connectNotConnected(ids[i], ids[i+1]); err != nil {
			return err
		}
	}
	return nil
}



func (net *Network) ConnectNodesRing(ids []enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	if ids == nil {
		ids = net.getUpNodeIDs()
	}
	l := len(ids)
	if l < 2 {
		return nil
	}
	if err := net.connectNodesChain(ids); err != nil {
		return err
	}
	return net.connectNotConnected(ids[l-1], ids[0])
}



func (net *Network) ConnectNodesStar(ids []enode.ID, center enode.ID) (err error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	if ids == nil {
		ids = net.getUpNodeIDs()
	}
	for _, id := range ids {
		if center == id {
			continue
		}
		if err := net.connectNotConnected(center, id); err != nil {
			return err
		}
	}
	return nil
}

func (net *Network) connectNotConnected(oneID, otherID enode.ID) error {
	return ignoreAlreadyConnectedErr(net.connect(oneID, otherID))
}

func ignoreAlreadyConnectedErr(err error) error {
	if err == nil || strings.Contains(err.Error(), "already connected") {
		return nil
	}
	return err
}

















package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const jsonIndent = "    "



type nodeSet map[enode.ID]nodeJSON

type nodeJSON struct {
	Seq uint64      `json:"seq"`
	N   *enode.Node `json:"record"`

	
	
	Score int `json:"score,omitempty"`
	
	FirstResponse time.Time `json:"firstResponse,omitempty"`
	LastResponse  time.Time `json:"lastResponse,omitempty"`
	
	LastCheck time.Time `json:"lastCheck,omitempty"`
}

func loadNodesJSON(file string) nodeSet {
	var nodes nodeSet
	if err := common.LoadJSON(file, &nodes); err != nil {
		exit(err)
	}
	return nodes
}

func writeNodesJSON(file string, nodes nodeSet) {
	nodesJSON, err := json.MarshalIndent(nodes, "", jsonIndent)
	if err != nil {
		exit(err)
	}
	if file == "-" {
		os.Stdout.Write(nodesJSON)
		return
	}
	if err := ioutil.WriteFile(file, nodesJSON, 0644); err != nil {
		exit(err)
	}
}

func (ns nodeSet) nodes() []*enode.Node {
	result := make([]*enode.Node, 0, len(ns))
	for _, n := range ns {
		result = append(result, n.N)
	}
	
	sort.Slice(result, func(i, j int) bool {
		return bytes.Compare(result[i].ID().Bytes(), result[j].ID().Bytes()) < 0
	})
	return result
}

func (ns nodeSet) add(nodes ...*enode.Node) {
	for _, n := range nodes {
		ns[n.ID()] = nodeJSON{Seq: n.Seq(), N: n}
	}
}

func (ns nodeSet) verify() error {
	for id, n := range ns {
		if n.N.ID() != id {
			return fmt.Errorf("invalid node %v: ID does not match ID %v in record", id, n.N.ID())
		}
		if n.N.Seq() != n.Seq {
			return fmt.Errorf("invalid node %v: 'seq' does not match seq %d from record", id, n.N.Seq())
		}
	}
	return nil
}

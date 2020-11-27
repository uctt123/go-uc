















package trie

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
)



var ErrNotRequested = errors.New("not requested")



var ErrAlreadyProcessed = errors.New("already processed")




const maxFetchesPerDepth = 16384


type request struct {
	path []byte      
	hash common.Hash 
	data []byte      
	code bool        

	parents []*request 
	deps    int        

	callback LeafCallback 
}


















type SyncPath [][]byte



func newSyncPath(path []byte) SyncPath {
	
	
	
	
	
	if len(path) < 64 {
		return SyncPath{hexToCompact(path)}
	}
	return SyncPath{hexToKeybytes(path[:64]), hexToCompact(path[64:])}
}


type SyncResult struct {
	Hash common.Hash 
	Data []byte      
}



type syncMemBatch struct {
	nodes map[common.Hash][]byte 
	codes map[common.Hash][]byte 
}


func newSyncMemBatch() *syncMemBatch {
	return &syncMemBatch{
		nodes: make(map[common.Hash][]byte),
		codes: make(map[common.Hash][]byte),
	}
}


func (batch *syncMemBatch) hasNode(hash common.Hash) bool {
	_, ok := batch.nodes[hash]
	return ok
}


func (batch *syncMemBatch) hasCode(hash common.Hash) bool {
	_, ok := batch.codes[hash]
	return ok
}




type Sync struct {
	database ethdb.KeyValueReader     
	membatch *syncMemBatch            
	nodeReqs map[common.Hash]*request 
	codeReqs map[common.Hash]*request 
	queue    *prque.Prque             
	fetches  map[int]int              
	bloom    *SyncBloom               
}


func NewSync(root common.Hash, database ethdb.KeyValueReader, callback LeafCallback, bloom *SyncBloom) *Sync {
	ts := &Sync{
		database: database,
		membatch: newSyncMemBatch(),
		nodeReqs: make(map[common.Hash]*request),
		codeReqs: make(map[common.Hash]*request),
		queue:    prque.New(nil),
		fetches:  make(map[int]int),
		bloom:    bloom,
	}
	ts.AddSubTrie(root, nil, common.Hash{}, callback)
	return ts
}


func (s *Sync) AddSubTrie(root common.Hash, path []byte, parent common.Hash, callback LeafCallback) {
	
	if root == emptyRoot {
		return
	}
	if s.membatch.hasNode(root) {
		return
	}
	if s.bloom == nil || s.bloom.Contains(root[:]) {
		
		
		
		blob := rawdb.ReadTrieNode(s.database, root)
		if len(blob) > 0 {
			return
		}
		
		bloomFaultMeter.Mark(1)
	}
	
	req := &request{
		path:     path,
		hash:     root,
		callback: callback,
	}
	
	if parent != (common.Hash{}) {
		ancestor := s.nodeReqs[parent]
		if ancestor == nil {
			panic(fmt.Sprintf("sub-trie ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	s.schedule(req)
}




func (s *Sync) AddCodeEntry(hash common.Hash, path []byte, parent common.Hash) {
	
	if hash == emptyState {
		return
	}
	if s.membatch.hasCode(hash) {
		return
	}
	if s.bloom == nil || s.bloom.Contains(hash[:]) {
		
		
		
		
		
		
		if blob := rawdb.ReadCodeWithPrefix(s.database, hash); len(blob) > 0 {
			return
		}
		
		bloomFaultMeter.Mark(1)
	}
	
	req := &request{
		path: path,
		hash: hash,
		code: true,
	}
	
	if parent != (common.Hash{}) {
		ancestor := s.nodeReqs[parent] 
		if ancestor == nil {
			panic(fmt.Sprintf("raw-entry ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	s.schedule(req)
}




func (s *Sync) Missing(max int) (nodes []common.Hash, paths []SyncPath, codes []common.Hash) {
	var (
		nodeHashes []common.Hash
		nodePaths  []SyncPath
		codeHashes []common.Hash
	)
	for !s.queue.Empty() && (max == 0 || len(nodeHashes)+len(codeHashes) < max) {
		
		item, prio := s.queue.Peek()

		
		depth := int(prio >> 56)
		if s.fetches[depth] > maxFetchesPerDepth {
			break
		}
		
		s.queue.Pop()
		s.fetches[depth]++

		hash := item.(common.Hash)
		if req, ok := s.nodeReqs[hash]; ok {
			nodeHashes = append(nodeHashes, hash)
			nodePaths = append(nodePaths, newSyncPath(req.path))
		} else {
			codeHashes = append(codeHashes, hash)
		}
	}
	return nodeHashes, nodePaths, codeHashes
}







func (s *Sync) Process(result SyncResult) error {
	
	if s.nodeReqs[result.Hash] == nil && s.codeReqs[result.Hash] == nil {
		return ErrNotRequested
	}
	
	var filled bool
	if req := s.codeReqs[result.Hash]; req != nil && req.data == nil {
		filled = true
		req.data = result.Data
		s.commit(req)
	}
	
	if req := s.nodeReqs[result.Hash]; req != nil && req.data == nil {
		filled = true
		
		node, err := decodeNode(result.Hash[:], result.Data)
		if err != nil {
			return err
		}
		req.data = result.Data

		
		requests, err := s.children(req, node)
		if err != nil {
			return err
		}
		if len(requests) == 0 && req.deps == 0 {
			s.commit(req)
		} else {
			req.deps += len(requests)
			for _, child := range requests {
				s.schedule(child)
			}
		}
	}
	if !filled {
		return ErrAlreadyProcessed
	}
	return nil
}



func (s *Sync) Commit(dbw ethdb.Batch) error {
	
	for key, value := range s.membatch.nodes {
		rawdb.WriteTrieNode(dbw, key, value)
		s.bloom.Add(key[:])
	}
	for key, value := range s.membatch.codes {
		rawdb.WriteCode(dbw, key, value)
		s.bloom.Add(key[:])
	}
	
	s.membatch = newSyncMemBatch()
	return nil
}


func (s *Sync) Pending() int {
	return len(s.nodeReqs) + len(s.codeReqs)
}




func (s *Sync) schedule(req *request) {
	var reqset = s.nodeReqs
	if req.code {
		reqset = s.codeReqs
	}
	
	if old, ok := reqset[req.hash]; ok {
		old.parents = append(old.parents, req.parents...)
		return
	}
	reqset[req.hash] = req

	
	
	
	
	
	prio := int64(len(req.path)) << 56 
	for i := 0; i < 14 && i < len(req.path); i++ {
		prio |= int64(15-req.path[i]) << (52 - i*4) 
	}
	s.queue.Push(req.hash, prio)
}



func (s *Sync) children(req *request, object node) ([]*request, error) {
	
	type child struct {
		path []byte
		node node
	}
	var children []child

	switch node := (object).(type) {
	case *shortNode:
		key := node.Key
		if hasTerm(key) {
			key = key[:len(key)-1]
		}
		children = []child{{
			node: node.Val,
			path: append(append([]byte(nil), req.path...), key...),
		}}
	case *fullNode:
		for i := 0; i < 17; i++ {
			if node.Children[i] != nil {
				children = append(children, child{
					node: node.Children[i],
					path: append(append([]byte(nil), req.path...), byte(i)),
				})
			}
		}
	default:
		panic(fmt.Sprintf("unknown node: %+v", node))
	}
	
	requests := make([]*request, 0, len(children))
	for _, child := range children {
		
		if req.callback != nil {
			if node, ok := (child.node).(valueNode); ok {
				if err := req.callback(child.path, node, req.hash); err != nil {
					return nil, err
				}
			}
		}
		
		if node, ok := (child.node).(hashNode); ok {
			
			hash := common.BytesToHash(node)
			if s.membatch.hasNode(hash) {
				continue
			}
			if s.bloom == nil || s.bloom.Contains(node) {
				
				
				
				if blob := rawdb.ReadTrieNode(s.database, common.BytesToHash(node)); len(blob) > 0 {
					continue
				}
				
				bloomFaultMeter.Mark(1)
			}
			
			requests = append(requests, &request{
				path:     child.path,
				hash:     hash,
				parents:  []*request{req},
				callback: req.callback,
			})
		}
	}
	return requests, nil
}




func (s *Sync) commit(req *request) (err error) {
	
	if req.code {
		s.membatch.codes[req.hash] = req.data
		delete(s.codeReqs, req.hash)
		s.fetches[len(req.path)]--
	} else {
		s.membatch.nodes[req.hash] = req.data
		delete(s.nodeReqs, req.hash)
		s.fetches[len(req.path)]--
	}
	
	for _, parent := range req.parents {
		parent.deps--
		if parent.deps == 0 {
			if err := s.commit(parent); err != nil {
				return err
			}
		}
	}
	return nil
}

















package downloader

import (
	"fmt"
	"hash"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie"
	"golang.org/x/crypto/sha3"
)



type stateReq struct {
	nItems    uint16                    
	trieTasks map[common.Hash]*trieTask 
	codeTasks map[common.Hash]*codeTask 
	timeout   time.Duration             
	timer     *time.Timer               
	peer      *peerConnection           
	delivered time.Time                 
	response  [][]byte                  
	dropped   bool                      
}


func (req *stateReq) timedOut() bool {
	return req.response == nil
}



type stateSyncStats struct {
	processed  uint64 
	duplicate  uint64 
	unexpected uint64 
	pending    uint64 
}


func (d *Downloader) syncState(root common.Hash) *stateSync {
	
	s := newStateSync(d, root)
	select {
	case d.stateSyncStart <- s:
		
		
		
		<-s.started
	case <-d.quitCh:
		s.err = errCancelStateFetch
		close(s.done)
	}
	return s
}



func (d *Downloader) stateFetcher() {
	for {
		select {
		case s := <-d.stateSyncStart:
			for next := s; next != nil; {
				next = d.runStateSync(next)
			}
		case <-d.stateCh:
			
		case <-d.quitCh:
			return
		}
	}
}



func (d *Downloader) runStateSync(s *stateSync) *stateSync {
	var (
		active   = make(map[string]*stateReq) 
		finished []*stateReq                  
		timeout  = make(chan *stateReq)       
	)
	
	log.Trace("State sync starting", "root", s.root)
	go s.run()
	defer s.Cancel()

	
	peerDrop := make(chan *peerConnection, 1024)
	peerSub := s.d.peers.SubscribePeerDrops(peerDrop)
	defer peerSub.Unsubscribe()

	for {
		
		var (
			deliverReq   *stateReq
			deliverReqCh chan *stateReq
		)
		if len(finished) > 0 {
			deliverReq = finished[0]
			deliverReqCh = s.deliver
		}

		select {
		
		case next := <-d.stateSyncStart:
			d.spindownStateSync(active, finished, timeout, peerDrop)
			return next

		case <-s.done:
			d.spindownStateSync(active, finished, timeout, peerDrop)
			return nil

		
		case deliverReqCh <- deliverReq:
			
			copy(finished, finished[1:])
			finished[len(finished)-1] = nil
			finished = finished[:len(finished)-1]

		
		case pack := <-d.stateCh:
			
			req := active[pack.PeerId()]
			if req == nil {
				log.Debug("Unrequested node data", "peer", pack.PeerId(), "len", pack.Items())
				continue
			}
			
			req.timer.Stop()
			req.response = pack.(*statePack).states
			req.delivered = time.Now()

			finished = append(finished, req)
			delete(active, pack.PeerId())

		
		case p := <-peerDrop:
			
			req := active[p.id]
			if req == nil {
				continue
			}
			
			req.timer.Stop()
			req.dropped = true
			req.delivered = time.Now()

			finished = append(finished, req)
			delete(active, p.id)

		
		case req := <-timeout:
			
			
			
			if active[req.peer.id] != req {
				continue
			}
			req.delivered = time.Now()
			
			finished = append(finished, req)
			delete(active, req.peer.id)

		
		case req := <-d.trackStateReq:
			
			
			
			
			
			
			if old := active[req.peer.id]; old != nil {
				log.Warn("Busy peer assigned new state fetch", "peer", old.peer.id)
				
				old.timer.Stop()
				old.dropped = true
				old.delivered = time.Now()
				finished = append(finished, old)
			}
			
			req.timer = time.AfterFunc(req.timeout, func() {
				timeout <- req
			})
			active[req.peer.id] = req
		}
	}
}




func (d *Downloader) spindownStateSync(active map[string]*stateReq, finished []*stateReq, timeout chan *stateReq, peerDrop chan *peerConnection) {
	log.Trace("State sync spinning down", "active", len(active), "finished", len(finished))
	for len(active) > 0 {
		var (
			req    *stateReq
			reason string
		)
		select {
		
		case pack := <-d.stateCh:
			req = active[pack.PeerId()]
			reason = "delivered"
		
		case p := <-peerDrop:
			req = active[p.id]
			reason = "peerdrop"
		
		case req = <-timeout:
			reason = "timeout"
		}
		if req == nil {
			continue
		}
		req.peer.log.Trace("State peer marked idle (spindown)", "req.items", int(req.nItems), "reason", reason)
		req.timer.Stop()
		delete(active, req.peer.id)
		req.peer.SetNodeDataIdle(int(req.nItems), time.Now())
	}
	
	
	
	for _, req := range finished {
		req.peer.SetNodeDataIdle(int(req.nItems), time.Now())
	}
}



type stateSync struct {
	d *Downloader 

	sched  *trie.Sync 
	keccak hash.Hash  

	trieTasks map[common.Hash]*trieTask 
	codeTasks map[common.Hash]*codeTask 

	numUncommitted   int
	bytesUncommitted int

	started chan struct{} 

	deliver    chan *stateReq 
	cancel     chan struct{}  
	cancelOnce sync.Once      
	done       chan struct{}  
	err        error          

	root common.Hash
}



type trieTask struct {
	path     [][]byte
	attempts map[string]struct{}
}



type codeTask struct {
	attempts map[string]struct{}
}



func newStateSync(d *Downloader, root common.Hash) *stateSync {
	return &stateSync{
		d:         d,
		sched:     state.NewStateSync(root, d.stateDB, d.stateBloom),
		keccak:    sha3.NewLegacyKeccak256(),
		trieTasks: make(map[common.Hash]*trieTask),
		codeTasks: make(map[common.Hash]*codeTask),
		deliver:   make(chan *stateReq),
		cancel:    make(chan struct{}),
		done:      make(chan struct{}),
		started:   make(chan struct{}),
		root:      root,
	}
}




func (s *stateSync) run() {
	s.err = s.loop()
	close(s.done)
}


func (s *stateSync) Wait() error {
	<-s.done
	return s.err
}


func (s *stateSync) Cancel() error {
	s.cancelOnce.Do(func() { close(s.cancel) })
	return s.Wait()
}







func (s *stateSync) loop() (err error) {
	close(s.started)
	
	newPeer := make(chan *peerConnection, 1024)
	peerSub := s.d.peers.SubscribeNewPeers(newPeer)
	defer peerSub.Unsubscribe()
	defer func() {
		cerr := s.commit(true)
		if err == nil {
			err = cerr
		}
	}()

	
	for s.sched.Pending() > 0 {
		if err = s.commit(false); err != nil {
			return err
		}
		s.assignTasks()
		
		select {
		case <-newPeer:
			

		case <-s.cancel:
			return errCancelStateFetch

		case <-s.d.cancelCh:
			return errCanceled

		case req := <-s.deliver:
			
			log.Trace("Received node data response", "peer", req.peer.id, "count", len(req.response), "dropped", req.dropped, "timeout", !req.dropped && req.timedOut())
			if req.nItems <= 2 && !req.dropped && req.timedOut() {
				
				
				log.Warn("Stalling state sync, dropping peer", "peer", req.peer.id)
				if s.d.dropPeer == nil {
					
					
					req.peer.log.Warn("Downloader wants to drop peer, but peerdrop-function is not set", "peer", req.peer.id)
				} else {
					s.d.dropPeer(req.peer.id)

					
					s.d.cancelLock.RLock()
					master := req.peer.id == s.d.cancelPeer
					s.d.cancelLock.RUnlock()

					if master {
						s.d.cancel()
						return errTimeout
					}
				}
			}
			
			delivered, err := s.process(req)
			req.peer.SetNodeDataIdle(delivered, req.delivered)
			if err != nil {
				log.Warn("Node data write error", "err", err)
				return err
			}
		}
	}
	return nil
}

func (s *stateSync) commit(force bool) error {
	if !force && s.bytesUncommitted < ethdb.IdealBatchSize {
		return nil
	}
	start := time.Now()
	b := s.d.stateDB.NewBatch()
	if err := s.sched.Commit(b); err != nil {
		return err
	}
	if err := b.Write(); err != nil {
		return fmt.Errorf("DB write error: %v", err)
	}
	s.updateStats(s.numUncommitted, 0, 0, time.Since(start))
	s.numUncommitted = 0
	s.bytesUncommitted = 0
	return nil
}



func (s *stateSync) assignTasks() {
	
	peers, _ := s.d.peers.NodeDataIdlePeers()
	for _, p := range peers {
		
		cap := p.NodeDataCapacity(s.d.requestRTT())
		req := &stateReq{peer: p, timeout: s.d.requestTTL()}

		nodes, _, codes := s.fillTasks(cap, req)

		
		if len(nodes)+len(codes) > 0 {
			req.peer.log.Trace("Requesting batch of state data", "nodes", len(nodes), "codes", len(codes), "root", s.root)
			select {
			case s.d.trackStateReq <- req:
				req.peer.FetchNodeData(append(nodes, codes...)) 
			case <-s.cancel:
			case <-s.d.cancelCh:
			}
		}
	}
}



func (s *stateSync) fillTasks(n int, req *stateReq) (nodes []common.Hash, paths []trie.SyncPath, codes []common.Hash) {
	
	if fill := n - (len(s.trieTasks) + len(s.codeTasks)); fill > 0 {
		nodes, paths, codes := s.sched.Missing(fill)
		for i, hash := range nodes {
			s.trieTasks[hash] = &trieTask{
				path:     paths[i],
				attempts: make(map[string]struct{}),
			}
		}
		for _, hash := range codes {
			s.codeTasks[hash] = &codeTask{
				attempts: make(map[string]struct{}),
			}
		}
	}
	
	
	nodes = make([]common.Hash, 0, n)
	paths = make([]trie.SyncPath, 0, n)
	codes = make([]common.Hash, 0, n)

	req.trieTasks = make(map[common.Hash]*trieTask, n)
	req.codeTasks = make(map[common.Hash]*codeTask, n)

	for hash, t := range s.codeTasks {
		
		if len(nodes)+len(codes) == n {
			break
		}
		
		if _, ok := t.attempts[req.peer.id]; ok {
			continue
		}
		
		t.attempts[req.peer.id] = struct{}{}
		codes = append(codes, hash)
		req.codeTasks[hash] = t
		delete(s.codeTasks, hash)
	}
	for hash, t := range s.trieTasks {
		
		if len(nodes)+len(codes) == n {
			break
		}
		
		if _, ok := t.attempts[req.peer.id]; ok {
			continue
		}
		
		t.attempts[req.peer.id] = struct{}{}

		nodes = append(nodes, hash)
		paths = append(paths, t.path)

		req.trieTasks[hash] = t
		delete(s.trieTasks, hash)
	}
	req.nItems = uint16(len(nodes) + len(codes))
	return nodes, paths, codes
}





func (s *stateSync) process(req *stateReq) (int, error) {
	
	duplicate, unexpected, successful := 0, 0, 0

	defer func(start time.Time) {
		if duplicate > 0 || unexpected > 0 {
			s.updateStats(0, duplicate, unexpected, time.Since(start))
		}
	}(time.Now())

	
	for _, blob := range req.response {
		hash, err := s.processNodeData(blob)
		switch err {
		case nil:
			s.numUncommitted++
			s.bytesUncommitted += len(blob)
			successful++
		case trie.ErrNotRequested:
			unexpected++
		case trie.ErrAlreadyProcessed:
			duplicate++
		default:
			return successful, fmt.Errorf("invalid state node %s: %v", hash.TerminalString(), err)
		}
		
		delete(req.trieTasks, hash)
		delete(req.codeTasks, hash)
	}
	
	npeers := s.d.peers.Len()
	for hash, task := range req.trieTasks {
		
		
		
		if len(req.response) > 0 || req.timedOut() {
			delete(task.attempts, req.peer.id)
		}
		
		
		if len(task.attempts) >= npeers {
			return successful, fmt.Errorf("trie node %s failed with all peers (%d tries, %d peers)", hash.TerminalString(), len(task.attempts), npeers)
		}
		
		s.trieTasks[hash] = task
	}
	for hash, task := range req.codeTasks {
		
		
		
		if len(req.response) > 0 || req.timedOut() {
			delete(task.attempts, req.peer.id)
		}
		
		
		if len(task.attempts) >= npeers {
			return successful, fmt.Errorf("byte code %s failed with all peers (%d tries, %d peers)", hash.TerminalString(), len(task.attempts), npeers)
		}
		
		s.codeTasks[hash] = task
	}
	return successful, nil
}




func (s *stateSync) processNodeData(blob []byte) (common.Hash, error) {
	res := trie.SyncResult{Data: blob}
	s.keccak.Reset()
	s.keccak.Write(blob)
	s.keccak.Sum(res.Hash[:0])
	err := s.sched.Process(res)
	return res.Hash, err
}



func (s *stateSync) updateStats(written, duplicate, unexpected int, duration time.Duration) {
	s.d.syncStatsLock.Lock()
	defer s.d.syncStatsLock.Unlock()

	s.d.syncStatsState.pending = uint64(s.sched.Pending())
	s.d.syncStatsState.processed += uint64(written)
	s.d.syncStatsState.duplicate += uint64(duplicate)
	s.d.syncStatsState.unexpected += uint64(unexpected)

	if written > 0 || duplicate > 0 || unexpected > 0 {
		log.Info("Imported new state entries", "count", written, "elapsed", common.PrettyDuration(duration), "processed", s.d.syncStatsState.processed, "pending", s.d.syncStatsState.pending, "trieretry", len(s.trieTasks), "coderetry", len(s.codeTasks), "duplicate", s.d.syncStatsState.duplicate, "unexpected", s.d.syncStatsState.unexpected)
	}
	if written > 0 {
		rawdb.WriteFastTrieProgress(s.d.stateDB, s.d.syncStatsState.processed)
	}
}

















package les

import (
	"errors"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/ethdb"
	lpc "github.com/ethereum/go-ethereum/les/lespay/client"
	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	minTimeout          = time.Millisecond * 500 
	timeoutRefresh      = time.Second * 5        
	dialCost            = 10000                  
	dialWaitStep        = 1.5                    
	queryCost           = 500                    
	queryWaitStep       = 1.02                   
	waitThreshold       = time.Hour * 2000       
	nodeWeightMul       = 1000000                
	nodeWeightThreshold = 100                    
	minRedialWait       = 10                     
	preNegLimit         = 5                      
	maxQueryFails       = 100                    
)



type serverPool struct {
	clock    mclock.Clock
	unixTime func() int64
	db       ethdb.KeyValueStore

	ns           *nodestate.NodeStateMachine
	vt           *lpc.ValueTracker
	mixer        *enode.FairMix
	mixSources   []enode.Iterator
	dialIterator enode.Iterator
	validSchemes enr.IdentityScheme
	trustedURLs  []string
	fillSet      *lpc.FillSet
	queryFails   uint32

	timeoutLock      sync.RWMutex
	timeout          time.Duration
	timeWeights      lpc.ResponseTimeWeights
	timeoutRefreshed mclock.AbsTime
}



type nodeHistory struct {
	dialCost                       utils.ExpiredValue
	redialWaitStart, redialWaitEnd int64 
}

type nodeHistoryEnc struct {
	DialCost                       utils.ExpiredValue
	RedialWaitStart, RedialWaitEnd uint64
}




type queryFunc func(*enode.Node) int

var (
	serverPoolSetup    = &nodestate.Setup{Version: 1}
	sfHasValue         = serverPoolSetup.NewPersistentFlag("hasValue")
	sfQueried          = serverPoolSetup.NewFlag("queried")
	sfCanDial          = serverPoolSetup.NewFlag("canDial")
	sfDialing          = serverPoolSetup.NewFlag("dialed")
	sfWaitDialTimeout  = serverPoolSetup.NewFlag("dialTimeout")
	sfConnected        = serverPoolSetup.NewFlag("connected")
	sfRedialWait       = serverPoolSetup.NewFlag("redialWait")
	sfAlwaysConnect    = serverPoolSetup.NewFlag("alwaysConnect")
	sfDisableSelection = nodestate.MergeFlags(sfQueried, sfCanDial, sfDialing, sfConnected, sfRedialWait)

	sfiNodeHistory = serverPoolSetup.NewPersistentField("nodeHistory", reflect.TypeOf(nodeHistory{}),
		func(field interface{}) ([]byte, error) {
			if n, ok := field.(nodeHistory); ok {
				ne := nodeHistoryEnc{
					DialCost:        n.dialCost,
					RedialWaitStart: uint64(n.redialWaitStart),
					RedialWaitEnd:   uint64(n.redialWaitEnd),
				}
				enc, err := rlp.EncodeToBytes(&ne)
				return enc, err
			} else {
				return nil, errors.New("invalid field type")
			}
		},
		func(enc []byte) (interface{}, error) {
			var ne nodeHistoryEnc
			err := rlp.DecodeBytes(enc, &ne)
			n := nodeHistory{
				dialCost:        ne.DialCost,
				redialWaitStart: int64(ne.RedialWaitStart),
				redialWaitEnd:   int64(ne.RedialWaitEnd),
			}
			return n, err
		},
	)
	sfiNodeWeight     = serverPoolSetup.NewField("nodeWeight", reflect.TypeOf(uint64(0)))
	sfiConnectedStats = serverPoolSetup.NewField("connectedStats", reflect.TypeOf(lpc.ResponseTimeStats{}))
)


func newServerPool(db ethdb.KeyValueStore, dbKey []byte, vt *lpc.ValueTracker, discovery enode.Iterator, mixTimeout time.Duration, query queryFunc, clock mclock.Clock, trustedURLs []string) *serverPool {
	s := &serverPool{
		db:           db,
		clock:        clock,
		unixTime:     func() int64 { return time.Now().Unix() },
		validSchemes: enode.ValidSchemes,
		trustedURLs:  trustedURLs,
		vt:           vt,
		ns:           nodestate.NewNodeStateMachine(db, []byte(string(dbKey)+"ns:"), clock, serverPoolSetup),
	}
	s.recalTimeout()
	s.mixer = enode.NewFairMix(mixTimeout)
	knownSelector := lpc.NewWrsIterator(s.ns, sfHasValue, sfDisableSelection, sfiNodeWeight)
	alwaysConnect := lpc.NewQueueIterator(s.ns, sfAlwaysConnect, sfDisableSelection, true, nil)
	s.mixSources = append(s.mixSources, knownSelector)
	s.mixSources = append(s.mixSources, alwaysConnect)
	if discovery != nil {
		s.mixSources = append(s.mixSources, discovery)
	}

	iter := enode.Iterator(s.mixer)
	if query != nil {
		iter = s.addPreNegFilter(iter, query)
	}
	s.dialIterator = enode.Filter(iter, func(node *enode.Node) bool {
		s.ns.SetState(node, sfDialing, sfCanDial, 0)
		s.ns.SetState(node, sfWaitDialTimeout, nodestate.Flags{}, time.Second*10)
		return true
	})

	s.ns.SubscribeState(nodestate.MergeFlags(sfWaitDialTimeout, sfConnected), func(n *enode.Node, oldState, newState nodestate.Flags) {
		if oldState.Equals(sfWaitDialTimeout) && newState.IsEmpty() {
			
			s.setRedialWait(n, dialCost, dialWaitStep)
			s.ns.SetStateSub(n, nodestate.Flags{}, sfDialing, 0)
		}
	})

	s.ns.AddLogMetrics(sfHasValue, sfDisableSelection, "selectable", nil, nil, serverSelectableGauge)
	s.ns.AddLogMetrics(sfDialing, nodestate.Flags{}, "dialed", serverDialedMeter, nil, nil)
	s.ns.AddLogMetrics(sfConnected, nodestate.Flags{}, "connected", nil, nil, serverConnectedGauge)
	return s
}




func (s *serverPool) addPreNegFilter(input enode.Iterator, query queryFunc) enode.Iterator {
	s.fillSet = lpc.NewFillSet(s.ns, input, sfQueried)
	s.ns.SubscribeState(sfQueried, func(n *enode.Node, oldState, newState nodestate.Flags) {
		if newState.Equals(sfQueried) {
			fails := atomic.LoadUint32(&s.queryFails)
			if fails == maxQueryFails {
				log.Warn("UDP pre-negotiation query does not seem to work")
			}
			if fails > maxQueryFails {
				fails = maxQueryFails
			}
			if rand.Intn(maxQueryFails*2) < int(fails) {
				
				
				s.ns.SetStateSub(n, sfCanDial, nodestate.Flags{}, time.Second*10)
				
				
				s.ns.SetStateSub(n, nodestate.Flags{}, sfQueried, 0)
				return
			}
			go func() {
				q := query(n)
				if q == -1 {
					atomic.AddUint32(&s.queryFails, 1)
				} else {
					atomic.StoreUint32(&s.queryFails, 0)
				}
				s.ns.Operation(func() {
					
					if q == 1 {
						s.ns.SetStateSub(n, sfCanDial, nodestate.Flags{}, time.Second*10)
					} else {
						s.setRedialWait(n, queryCost, queryWaitStep)
					}
					s.ns.SetStateSub(n, nodestate.Flags{}, sfQueried, 0)
				})
			}()
		}
	})
	return lpc.NewQueueIterator(s.ns, sfCanDial, nodestate.Flags{}, false, func(waiting bool) {
		if waiting {
			s.fillSet.SetTarget(preNegLimit)
		} else {
			s.fillSet.SetTarget(0)
		}
	})
}


func (s *serverPool) start() {
	s.ns.Start()
	for _, iter := range s.mixSources {
		
		
		s.mixer.AddSource(iter)
	}
	for _, url := range s.trustedURLs {
		if node, err := enode.Parse(s.validSchemes, url); err == nil {
			s.ns.SetState(node, sfAlwaysConnect, nodestate.Flags{}, 0)
		} else {
			log.Error("Invalid trusted server URL", "url", url, "error", err)
		}
	}
	unixTime := s.unixTime()
	s.ns.Operation(func() {
		s.ns.ForEach(sfHasValue, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
			s.calculateWeight(node)
			if n, ok := s.ns.GetField(node, sfiNodeHistory).(nodeHistory); ok && n.redialWaitEnd > unixTime {
				wait := n.redialWaitEnd - unixTime
				lastWait := n.redialWaitEnd - n.redialWaitStart
				if wait > lastWait {
					
					
					wait = lastWait
				}
				s.ns.SetStateSub(node, sfRedialWait, nodestate.Flags{}, time.Duration(wait)*time.Second)
			}
		})
	})
}


func (s *serverPool) stop() {
	s.dialIterator.Close()
	if s.fillSet != nil {
		s.fillSet.Close()
	}
	s.ns.Operation(func() {
		s.ns.ForEach(sfConnected, nodestate.Flags{}, func(n *enode.Node, state nodestate.Flags) {
			
			s.calculateWeight(n)
		})
	})
	s.ns.Stop()
}


func (s *serverPool) registerPeer(p *serverPeer) {
	s.ns.SetState(p.Node(), sfConnected, sfDialing.Or(sfWaitDialTimeout), 0)
	nvt := s.vt.Register(p.ID())
	s.ns.SetField(p.Node(), sfiConnectedStats, nvt.RtStats())
	p.setValueTracker(s.vt, nvt)
	p.updateVtParams()
}


func (s *serverPool) unregisterPeer(p *serverPeer) {
	s.ns.Operation(func() {
		s.setRedialWait(p.Node(), dialCost, dialWaitStep)
		s.ns.SetStateSub(p.Node(), nodestate.Flags{}, sfConnected, 0)
		s.ns.SetFieldSub(p.Node(), sfiConnectedStats, nil)
	})
	s.vt.Unregister(p.ID())
	p.setValueTracker(nil, nil)
}




func (s *serverPool) recalTimeout() {
	
	s.timeoutLock.RLock()
	refreshed := s.timeoutRefreshed
	s.timeoutLock.RUnlock()
	now := s.clock.Now()
	if refreshed != 0 && time.Duration(now-refreshed) < timeoutRefresh {
		return
	}
	
	rts := s.vt.RtStats()

	
	
	
	rts.Add(time.Second*2, 10, s.vt.StatsExpFactor())

	
	
	timeout := minTimeout
	if t := rts.Timeout(0.1); t > timeout {
		timeout = t
	}
	if t := rts.Timeout(0.5) * 2; t > timeout {
		timeout = t
	}
	s.timeoutLock.Lock()
	if s.timeout != timeout {
		s.timeout = timeout
		s.timeWeights = lpc.TimeoutWeights(s.timeout)

		suggestedTimeoutGauge.Update(int64(s.timeout / time.Millisecond))
		totalValueGauge.Update(int64(rts.Value(s.timeWeights, s.vt.StatsExpFactor())))
	}
	s.timeoutRefreshed = now
	s.timeoutLock.Unlock()
}


func (s *serverPool) getTimeout() time.Duration {
	s.recalTimeout()
	s.timeoutLock.RLock()
	defer s.timeoutLock.RUnlock()
	return s.timeout
}



func (s *serverPool) getTimeoutAndWeight() (time.Duration, lpc.ResponseTimeWeights) {
	s.recalTimeout()
	s.timeoutLock.RLock()
	defer s.timeoutLock.RUnlock()
	return s.timeout, s.timeWeights
}



func (s *serverPool) addDialCost(n *nodeHistory, amount int64) uint64 {
	logOffset := s.vt.StatsExpirer().LogOffset(s.clock.Now())
	if amount > 0 {
		n.dialCost.Add(amount, logOffset)
	}
	totalDialCost := n.dialCost.Value(logOffset)
	if totalDialCost < dialCost {
		totalDialCost = dialCost
	}
	return totalDialCost
}


func (s *serverPool) serviceValue(node *enode.Node) (sessionValue, totalValue float64) {
	nvt := s.vt.GetNode(node.ID())
	if nvt == nil {
		return 0, 0
	}
	currentStats := nvt.RtStats()
	_, timeWeights := s.getTimeoutAndWeight()
	expFactor := s.vt.StatsExpFactor()

	totalValue = currentStats.Value(timeWeights, expFactor)
	if connStats, ok := s.ns.GetField(node, sfiConnectedStats).(lpc.ResponseTimeStats); ok {
		diff := currentStats
		diff.SubStats(&connStats)
		sessionValue = diff.Value(timeWeights, expFactor)
		sessionValueMeter.Mark(int64(sessionValue))
	}
	return
}




func (s *serverPool) updateWeight(node *enode.Node, totalValue float64, totalDialCost uint64) {
	weight := uint64(totalValue * nodeWeightMul / float64(totalDialCost))
	if weight >= nodeWeightThreshold {
		s.ns.SetStateSub(node, sfHasValue, nodestate.Flags{}, 0)
		s.ns.SetFieldSub(node, sfiNodeWeight, weight)
	} else {
		s.ns.SetStateSub(node, nodestate.Flags{}, sfHasValue, 0)
		s.ns.SetFieldSub(node, sfiNodeWeight, nil)
		s.ns.SetFieldSub(node, sfiNodeHistory, nil)
	}
	s.ns.Persist(node) 
}










func (s *serverPool) setRedialWait(node *enode.Node, addDialCost int64, waitStep float64) {
	n, _ := s.ns.GetField(node, sfiNodeHistory).(nodeHistory)
	sessionValue, totalValue := s.serviceValue(node)
	totalDialCost := s.addDialCost(&n, addDialCost)

	
	
	
	
	
	
	
	
	
	unixTime := s.unixTime()
	plannedTimeout := float64(n.redialWaitEnd - n.redialWaitStart) 
	var actualWait float64                                         
	if unixTime > n.redialWaitEnd {
		
		actualWait = plannedTimeout
	} else {
		
		
		
		
		
		
		
		actualWait = float64(unixTime - n.redialWaitStart)
	}
	
	
	nextTimeout := actualWait * waitStep
	if plannedTimeout > nextTimeout {
		nextTimeout = plannedTimeout
	}
	
	
	a := totalValue * dialCost * float64(minRedialWait)
	b := float64(totalDialCost) * sessionValue
	if a < b*nextTimeout {
		nextTimeout = a / b
	}
	if nextTimeout < minRedialWait {
		nextTimeout = minRedialWait
	}
	wait := time.Duration(float64(time.Second) * nextTimeout)
	if wait < waitThreshold {
		n.redialWaitStart = unixTime
		n.redialWaitEnd = unixTime + int64(nextTimeout)
		s.ns.SetFieldSub(node, sfiNodeHistory, n)
		s.ns.SetStateSub(node, sfRedialWait, nodestate.Flags{}, wait)
		s.updateWeight(node, totalValue, totalDialCost)
	} else {
		
		
		s.ns.SetFieldSub(node, sfiNodeHistory, nil)
		s.ns.SetFieldSub(node, sfiNodeWeight, nil)
		s.ns.SetStateSub(node, nodestate.Flags{}, sfHasValue, 0)
	}
}





func (s *serverPool) calculateWeight(node *enode.Node) {
	n, _ := s.ns.GetField(node, sfiNodeHistory).(nodeHistory)
	_, totalValue := s.serviceValue(node)
	totalDialCost := s.addDialCost(&n, 0)
	s.updateWeight(node, totalValue, totalDialCost)
}

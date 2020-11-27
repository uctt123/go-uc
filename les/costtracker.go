















package les

import (
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/les/flowcontrol"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

const makeCostStats = false 

var (
	
	reqAvgTimeCost = requestCostTable{
		GetBlockHeadersMsg:     {150000, 30000},
		GetBlockBodiesMsg:      {0, 700000},
		GetReceiptsMsg:         {0, 1000000},
		GetCodeMsg:             {0, 450000},
		GetProofsV2Msg:         {0, 600000},
		GetHelperTrieProofsMsg: {0, 1000000},
		SendTxV2Msg:            {0, 450000},
		GetTxStatusMsg:         {0, 250000},
	}
	
	reqMaxInSize = requestCostTable{
		GetBlockHeadersMsg:     {40, 0},
		GetBlockBodiesMsg:      {0, 40},
		GetReceiptsMsg:         {0, 40},
		GetCodeMsg:             {0, 80},
		GetProofsV2Msg:         {0, 80},
		GetHelperTrieProofsMsg: {0, 20},
		SendTxV2Msg:            {0, 16500},
		GetTxStatusMsg:         {0, 50},
	}
	
	reqMaxOutSize = requestCostTable{
		GetBlockHeadersMsg:     {0, 556},
		GetBlockBodiesMsg:      {0, 100000},
		GetReceiptsMsg:         {0, 200000},
		GetCodeMsg:             {0, 50000},
		GetProofsV2Msg:         {0, 4000},
		GetHelperTrieProofsMsg: {0, 4000},
		SendTxV2Msg:            {0, 100},
		GetTxStatusMsg:         {0, 100},
	}
	
	minBufferReqAmount = map[uint64]uint64{
		GetBlockHeadersMsg:     192,
		GetBlockBodiesMsg:      1,
		GetReceiptsMsg:         1,
		GetCodeMsg:             1,
		GetProofsV2Msg:         1,
		GetHelperTrieProofsMsg: 16,
		SendTxV2Msg:            8,
		GetTxStatusMsg:         64,
	}
	minBufferMultiplier = 3
)

const (
	maxCostFactor    = 2    
	bufLimitRatio    = 6000 
	gfUsageThreshold = 0.5
	gfUsageTC        = time.Second
	gfRaiseTC        = time.Second * 200
	gfDropTC         = time.Second * 50
	gfDbKey          = "_globalCostFactorV6"
)
























type costTracker struct {
	db     ethdb.Database
	stopCh chan chan struct{}

	inSizeFactor  float64
	outSizeFactor float64
	factor        float64
	utilTarget    float64
	minBufLimit   uint64

	gfLock          sync.RWMutex
	reqInfoCh       chan reqInfo
	totalRechargeCh chan uint64

	stats map[uint64][]uint64 

	
	testing      bool            
	testCostList RequestCostList 
}



func newCostTracker(db ethdb.Database, config *eth.Config) (*costTracker, uint64) {
	utilTarget := float64(config.LightServ) * flowcontrol.FixedPointMultiplier / 100
	ct := &costTracker{
		db:         db,
		stopCh:     make(chan chan struct{}),
		reqInfoCh:  make(chan reqInfo, 100),
		utilTarget: utilTarget,
	}
	if config.LightIngress > 0 {
		ct.inSizeFactor = utilTarget / float64(config.LightIngress)
	}
	if config.LightEgress > 0 {
		ct.outSizeFactor = utilTarget / float64(config.LightEgress)
	}
	if makeCostStats {
		ct.stats = make(map[uint64][]uint64)
		for code := range reqAvgTimeCost {
			ct.stats[code] = make([]uint64, 10)
		}
	}
	ct.gfLoop()
	costList := ct.makeCostList(ct.globalFactor() * 1.25)
	for _, c := range costList {
		amount := minBufferReqAmount[c.MsgCode]
		cost := c.BaseCost + amount*c.ReqCost
		if cost > ct.minBufLimit {
			ct.minBufLimit = cost
		}
	}
	ct.minBufLimit *= uint64(minBufferMultiplier)
	return ct, (ct.minBufLimit-1)/bufLimitRatio + 1
}


func (ct *costTracker) stop() {
	stopCh := make(chan struct{})
	ct.stopCh <- stopCh
	<-stopCh
	if makeCostStats {
		ct.printStats()
	}
}



func (ct *costTracker) makeCostList(globalFactor float64) RequestCostList {
	maxCost := func(avgTimeCost, inSize, outSize uint64) uint64 {
		cost := avgTimeCost * maxCostFactor
		inSizeCost := uint64(float64(inSize) * ct.inSizeFactor * globalFactor)
		if inSizeCost > cost {
			cost = inSizeCost
		}
		outSizeCost := uint64(float64(outSize) * ct.outSizeFactor * globalFactor)
		if outSizeCost > cost {
			cost = outSizeCost
		}
		return cost
	}
	var list RequestCostList
	for code, data := range reqAvgTimeCost {
		baseCost := maxCost(data.baseCost, reqMaxInSize[code].baseCost, reqMaxOutSize[code].baseCost)
		reqCost := maxCost(data.reqCost, reqMaxInSize[code].reqCost, reqMaxOutSize[code].reqCost)
		if ct.minBufLimit != 0 {
			
			maxCost := baseCost + reqCost*minBufferReqAmount[code]
			if maxCost > ct.minBufLimit {
				mul := 0.999 * float64(ct.minBufLimit) / float64(maxCost)
				baseCost = uint64(float64(baseCost) * mul)
				reqCost = uint64(float64(reqCost) * mul)
			}
		}

		list = append(list, requestCostListItem{
			MsgCode:  code,
			BaseCost: baseCost,
			ReqCost:  reqCost,
		})
	}
	return list
}



type reqInfo struct {
	
	avgTimeCost float64

	
	
	servingTime float64

	
	msgCode uint64
}










func (ct *costTracker) gfLoop() {
	var (
		factor, totalRecharge        float64
		gfLog, recentTime, recentAvg float64

		lastUpdate, expUpdate = mclock.Now(), mclock.Now()
	)

	
	data, _ := ct.db.Get([]byte(gfDbKey))
	if len(data) == 8 {
		gfLog = math.Float64frombits(binary.BigEndian.Uint64(data[:]))
	}
	ct.factor = math.Exp(gfLog)
	factor, totalRecharge = ct.factor, ct.utilTarget*ct.factor

	
	
	threshold := gfUsageThreshold * float64(gfUsageTC) * ct.utilTarget / flowcontrol.FixedPointMultiplier

	go func() {
		saveCostFactor := func() {
			var data [8]byte
			binary.BigEndian.PutUint64(data[:], math.Float64bits(gfLog))
			ct.db.Put([]byte(gfDbKey), data[:])
			log.Debug("global cost factor saved", "value", factor)
		}
		saveTicker := time.NewTicker(time.Minute * 10)
		defer saveTicker.Stop()

		for {
			select {
			case r := <-ct.reqInfoCh:
				relCost := int64(factor * r.servingTime * 100 / r.avgTimeCost) 

				
				if metrics.EnabledExpensive {
					switch r.msgCode {
					case GetBlockHeadersMsg:
						relativeCostHeaderHistogram.Update(relCost)
					case GetBlockBodiesMsg:
						relativeCostBodyHistogram.Update(relCost)
					case GetReceiptsMsg:
						relativeCostReceiptHistogram.Update(relCost)
					case GetCodeMsg:
						relativeCostCodeHistogram.Update(relCost)
					case GetProofsV2Msg:
						relativeCostProofHistogram.Update(relCost)
					case GetHelperTrieProofsMsg:
						relativeCostHelperProofHistogram.Update(relCost)
					case SendTxV2Msg:
						relativeCostSendTxHistogram.Update(relCost)
					case GetTxStatusMsg:
						relativeCostTxStatusHistogram.Update(relCost)
					}
				}
				
				
				
				
				
				
				if r.msgCode == SendTxV2Msg || r.msgCode == GetTxStatusMsg {
					continue
				}
				requestServedMeter.Mark(int64(r.servingTime))
				requestServedTimer.Update(time.Duration(r.servingTime))
				requestEstimatedMeter.Mark(int64(r.avgTimeCost / factor))
				requestEstimatedTimer.Update(time.Duration(r.avgTimeCost / factor))
				relativeCostHistogram.Update(relCost)

				now := mclock.Now()
				dt := float64(now - expUpdate)
				expUpdate = now
				exp := math.Exp(-dt / float64(gfUsageTC))

				
				var gfCorr float64
				max := recentTime
				if recentAvg > max {
					max = recentAvg
				}
				
				if max > threshold {
					
					if max*exp >= threshold {
						gfCorr = dt
					} else {
						gfCorr = math.Log(max/threshold) * float64(gfUsageTC)
					}
					
					if recentTime > recentAvg {
						
						gfCorr /= -float64(gfDropTC)
					} else {
						
						gfCorr /= float64(gfRaiseTC)
					}
				}
				
				recentTime = recentTime*exp + r.servingTime
				recentAvg = recentAvg*exp + r.avgTimeCost/factor

				if gfCorr != 0 {
					
					gfLog += gfCorr
					factor = math.Exp(gfLog)
					
					if time.Duration(now-lastUpdate) > time.Second {
						totalRecharge, lastUpdate = ct.utilTarget*factor, now
						ct.gfLock.Lock()
						ct.factor = factor
						ch := ct.totalRechargeCh
						ct.gfLock.Unlock()
						if ch != nil {
							select {
							case ct.totalRechargeCh <- uint64(totalRecharge):
							default:
							}
						}
						globalFactorGauge.Update(int64(1000 * factor))
						log.Debug("global cost factor updated", "factor", factor)
					}
				}
				recentServedGauge.Update(int64(recentTime))
				recentEstimatedGauge.Update(int64(recentAvg))

			case <-saveTicker.C:
				saveCostFactor()

			case stopCh := <-ct.stopCh:
				saveCostFactor()
				close(stopCh)
				return
			}
		}
	}()
}


func (ct *costTracker) globalFactor() float64 {
	ct.gfLock.RLock()
	defer ct.gfLock.RUnlock()

	return ct.factor
}



func (ct *costTracker) totalRecharge() uint64 {
	ct.gfLock.RLock()
	defer ct.gfLock.RUnlock()

	return uint64(ct.factor * ct.utilTarget)
}



func (ct *costTracker) subscribeTotalRecharge(ch chan uint64) uint64 {
	ct.gfLock.Lock()
	defer ct.gfLock.Unlock()

	ct.totalRechargeCh = ch
	return uint64(ct.factor * ct.utilTarget)
}



func (ct *costTracker) updateStats(code, amount, servingTime, realCost uint64) {
	avg := reqAvgTimeCost[code]
	avgTimeCost := avg.baseCost + amount*avg.reqCost
	select {
	case ct.reqInfoCh <- reqInfo{float64(avgTimeCost), float64(servingTime), code}:
	default:
	}
	if makeCostStats {
		realCost <<= 4
		l := 0
		for l < 9 && realCost > avgTimeCost {
			l++
			realCost >>= 1
		}
		atomic.AddUint64(&ct.stats[code][l], 1)
	}
}









func (ct *costTracker) realCost(servingTime uint64, inSize, outSize uint32) uint64 {
	cost := float64(servingTime)
	inSizeCost := float64(inSize) * ct.inSizeFactor
	if inSizeCost > cost {
		cost = inSizeCost
	}
	outSizeCost := float64(outSize) * ct.outSizeFactor
	if outSizeCost > cost {
		cost = outSizeCost
	}
	return uint64(cost * ct.globalFactor())
}


func (ct *costTracker) printStats() {
	if ct.stats == nil {
		return
	}
	for code, arr := range ct.stats {
		log.Info("Request cost statistics", "code", code, "1/16", arr[0], "1/8", arr[1], "1/4", arr[2], "1/2", arr[3], "1", arr[4], "2", arr[5], "4", arr[6], "8", arr[7], "16", arr[8], ">16", arr[9])
	}
}

type (
	
	
	
	requestCostTable map[uint64]*requestCosts
	requestCosts     struct {
		baseCost, reqCost uint64
	}

	
	
	RequestCostList     []requestCostListItem
	requestCostListItem struct {
		MsgCode, BaseCost, ReqCost uint64
	}
)


func (table requestCostTable) getMaxCost(code, amount uint64) uint64 {
	costs := table[code]
	return costs.baseCost + amount*costs.reqCost
}


func (list RequestCostList) decode(protocolLength uint64) requestCostTable {
	table := make(requestCostTable)
	for _, e := range list {
		if e.MsgCode < protocolLength {
			table[e.MsgCode] = &requestCosts{
				baseCost: e.BaseCost,
				reqCost:  e.ReqCost,
			}
		}
	}
	return table
}


func testCostList(testCost uint64) RequestCostList {
	cl := make(RequestCostList, len(reqAvgTimeCost))
	var max uint64
	for code := range reqAvgTimeCost {
		if code > max {
			max = code
		}
	}
	i := 0
	for code := uint64(0); code <= max; code++ {
		if _, ok := reqAvgTimeCost[code]; ok {
			cl[i].MsgCode = code
			cl[i].BaseCost = testCost
			cl[i].ReqCost = 0
			i++
		}
	}
	return cl
}

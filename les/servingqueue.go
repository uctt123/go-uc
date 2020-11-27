















package les

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/common/prque"
)



type servingQueue struct {
	recentTime, queuedTime, servingTimeDiff uint64
	burstLimit, burstDropLimit              uint64
	burstDecRate                            float64
	lastUpdate                              mclock.AbsTime

	queueAddCh, queueBestCh chan *servingTask
	stopThreadCh, quit      chan struct{}
	setThreadsCh            chan int

	wg          sync.WaitGroup
	threadCount int          
	queue       *prque.Prque 
	best        *servingTask 
	suspendBias int64        
}









type servingTask struct {
	sq                                       *servingQueue
	servingTime, timeAdded, maxTime, expTime uint64
	peer                                     *clientPeer
	priority                                 int64
	biasAdded                                bool
	token                                    runToken
	tokenCh                                  chan runToken
}




type runToken chan struct{}



func (t *servingTask) start() bool {
	if t.peer.isFrozen() {
		return false
	}
	t.tokenCh = make(chan runToken, 1)
	select {
	case t.sq.queueAddCh <- t:
	case <-t.sq.quit:
		return false
	}
	select {
	case t.token = <-t.tokenCh:
	case <-t.sq.quit:
		return false
	}
	if t.token == nil {
		return false
	}
	t.servingTime -= uint64(mclock.Now())
	return true
}



func (t *servingTask) done() uint64 {
	t.servingTime += uint64(mclock.Now())
	close(t.token)
	diff := t.servingTime - t.timeAdded
	t.timeAdded = t.servingTime
	if t.expTime > diff {
		t.expTime -= diff
		atomic.AddUint64(&t.sq.servingTimeDiff, t.expTime)
	} else {
		t.expTime = 0
	}
	return t.servingTime
}





func (t *servingTask) waitOrStop() bool {
	t.done()
	if !t.biasAdded {
		t.priority += t.sq.suspendBias
		t.biasAdded = true
	}
	return t.start()
}


func newServingQueue(suspendBias int64, utilTarget float64) *servingQueue {
	sq := &servingQueue{
		queue:          prque.New(nil),
		suspendBias:    suspendBias,
		queueAddCh:     make(chan *servingTask, 100),
		queueBestCh:    make(chan *servingTask),
		stopThreadCh:   make(chan struct{}),
		quit:           make(chan struct{}),
		setThreadsCh:   make(chan int, 10),
		burstLimit:     uint64(utilTarget * bufLimitRatio * 1200000),
		burstDropLimit: uint64(utilTarget * bufLimitRatio * 1000000),
		burstDecRate:   utilTarget,
		lastUpdate:     mclock.Now(),
	}
	sq.wg.Add(2)
	go sq.queueLoop()
	go sq.threadCountLoop()
	return sq
}


func (sq *servingQueue) newTask(peer *clientPeer, maxTime uint64, priority int64) *servingTask {
	return &servingTask{
		sq:       sq,
		peer:     peer,
		maxTime:  maxTime,
		expTime:  maxTime,
		priority: priority,
	}
}







func (sq *servingQueue) threadController() {
	for {
		token := make(runToken)
		select {
		case best := <-sq.queueBestCh:
			best.tokenCh <- token
		case <-sq.stopThreadCh:
			sq.wg.Done()
			return
		case <-sq.quit:
			sq.wg.Done()
			return
		}
		<-token
		select {
		case <-sq.stopThreadCh:
			sq.wg.Done()
			return
		case <-sq.quit:
			sq.wg.Done()
			return
		default:
		}
	}
}

type (
	
	peerTasks struct {
		peer     *clientPeer
		list     []*servingTask
		sumTime  uint64
		priority float64
	}
	
	peerList []*peerTasks
)

func (l peerList) Len() int {
	return len(l)
}

func (l peerList) Less(i, j int) bool {
	return l[i].priority < l[j].priority
}

func (l peerList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}



func (sq *servingQueue) freezePeers() {
	peerMap := make(map[*clientPeer]*peerTasks)
	var peerList peerList
	if sq.best != nil {
		sq.queue.Push(sq.best, sq.best.priority)
	}
	sq.best = nil
	for sq.queue.Size() > 0 {
		task := sq.queue.PopItem().(*servingTask)
		tasks := peerMap[task.peer]
		if tasks == nil {
			bufValue, bufLimit := task.peer.fcClient.BufferStatus()
			if bufLimit < 1 {
				bufLimit = 1
			}
			tasks = &peerTasks{
				peer:     task.peer,
				priority: float64(bufValue) / float64(bufLimit), 
			}
			peerMap[task.peer] = tasks
			peerList = append(peerList, tasks)
		}
		tasks.list = append(tasks.list, task)
		tasks.sumTime += task.expTime
	}
	sort.Sort(peerList)
	drop := true
	for _, tasks := range peerList {
		if drop {
			tasks.peer.freeze()
			tasks.peer.fcClient.Freeze()
			sq.queuedTime -= tasks.sumTime
			sqQueuedGauge.Update(int64(sq.queuedTime))
			clientFreezeMeter.Mark(1)
			drop = sq.recentTime+sq.queuedTime > sq.burstDropLimit
			for _, task := range tasks.list {
				task.tokenCh <- nil
			}
		} else {
			for _, task := range tasks.list {
				sq.queue.Push(task, task.priority)
			}
		}
	}
	if sq.queue.Size() > 0 {
		sq.best = sq.queue.PopItem().(*servingTask)
	}
}


func (sq *servingQueue) updateRecentTime() {
	subTime := atomic.SwapUint64(&sq.servingTimeDiff, 0)
	now := mclock.Now()
	dt := now - sq.lastUpdate
	sq.lastUpdate = now
	if dt > 0 {
		subTime += uint64(float64(dt) * sq.burstDecRate)
	}
	if sq.recentTime > subTime {
		sq.recentTime -= subTime
	} else {
		sq.recentTime = 0
	}
}


func (sq *servingQueue) addTask(task *servingTask) {
	if sq.best == nil {
		sq.best = task
	} else if task.priority > sq.best.priority {
		sq.queue.Push(sq.best, sq.best.priority)
		sq.best = task
	} else {
		sq.queue.Push(task, task.priority)
	}
	sq.updateRecentTime()
	sq.queuedTime += task.expTime
	sqServedGauge.Update(int64(sq.recentTime))
	sqQueuedGauge.Update(int64(sq.queuedTime))
	if sq.recentTime+sq.queuedTime > sq.burstLimit {
		sq.freezePeers()
	}
}




func (sq *servingQueue) queueLoop() {
	for {
		if sq.best != nil {
			expTime := sq.best.expTime
			select {
			case task := <-sq.queueAddCh:
				sq.addTask(task)
			case sq.queueBestCh <- sq.best:
				sq.updateRecentTime()
				sq.queuedTime -= expTime
				sq.recentTime += expTime
				sqServedGauge.Update(int64(sq.recentTime))
				sqQueuedGauge.Update(int64(sq.queuedTime))
				if sq.queue.Size() == 0 {
					sq.best = nil
				} else {
					sq.best, _ = sq.queue.PopItem().(*servingTask)
				}
			case <-sq.quit:
				sq.wg.Done()
				return
			}
		} else {
			select {
			case task := <-sq.queueAddCh:
				sq.addTask(task)
			case <-sq.quit:
				sq.wg.Done()
				return
			}
		}
	}
}



func (sq *servingQueue) threadCountLoop() {
	var threadCountTarget int
	for {
		for threadCountTarget > sq.threadCount {
			sq.wg.Add(1)
			go sq.threadController()
			sq.threadCount++
		}
		if threadCountTarget < sq.threadCount {
			select {
			case threadCountTarget = <-sq.setThreadsCh:
			case sq.stopThreadCh <- struct{}{}:
				sq.threadCount--
			case <-sq.quit:
				sq.wg.Done()
				return
			}
		} else {
			select {
			case threadCountTarget = <-sq.setThreadsCh:
			case <-sq.quit:
				sq.wg.Done()
				return
			}
		}
	}
}



func (sq *servingQueue) setThreads(threadCount int) {
	select {
	case sq.setThreadsCh <- threadCount:
	case <-sq.quit:
		return
	}
}


func (sq *servingQueue) stop() {
	close(sq.quit)
	sq.wg.Wait()
}

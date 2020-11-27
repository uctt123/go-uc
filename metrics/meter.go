package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)



type Meter interface {
	Count() int64
	Mark(int64)
	Rate1() float64
	Rate5() float64
	Rate15() float64
	RateMean() float64
	Snapshot() Meter
	Stop()
}





func GetOrRegisterMeter(name string, r Registry) Meter {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewMeter).(Meter)
}





func GetOrRegisterMeterForced(name string, r Registry) Meter {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewMeterForced).(Meter)
}



func NewMeter() Meter {
	if !Enabled {
		return NilMeter{}
	}
	m := newStandardMeter()
	arbiter.Lock()
	defer arbiter.Unlock()
	arbiter.meters[m] = struct{}{}
	if !arbiter.started {
		arbiter.started = true
		go arbiter.tick()
	}
	return m
}




func NewMeterForced() Meter {
	m := newStandardMeter()
	arbiter.Lock()
	defer arbiter.Unlock()
	arbiter.meters[m] = struct{}{}
	if !arbiter.started {
		arbiter.started = true
		go arbiter.tick()
	}
	return m
}





func NewRegisteredMeter(name string, r Registry) Meter {
	c := NewMeter()
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}





func NewRegisteredMeterForced(name string, r Registry) Meter {
	c := NewMeterForced()
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}


type MeterSnapshot struct {
	
	
	
	
	temp                           int64
	count                          int64
	rate1, rate5, rate15, rateMean float64
}


func (m *MeterSnapshot) Count() int64 { return m.count }


func (*MeterSnapshot) Mark(n int64) {
	panic("Mark called on a MeterSnapshot")
}



func (m *MeterSnapshot) Rate1() float64 { return m.rate1 }



func (m *MeterSnapshot) Rate5() float64 { return m.rate5 }



func (m *MeterSnapshot) Rate15() float64 { return m.rate15 }



func (m *MeterSnapshot) RateMean() float64 { return m.rateMean }


func (m *MeterSnapshot) Snapshot() Meter { return m }


func (m *MeterSnapshot) Stop() {}


type NilMeter struct{}


func (NilMeter) Count() int64 { return 0 }


func (NilMeter) Mark(n int64) {}


func (NilMeter) Rate1() float64 { return 0.0 }


func (NilMeter) Rate5() float64 { return 0.0 }


func (NilMeter) Rate15() float64 { return 0.0 }


func (NilMeter) RateMean() float64 { return 0.0 }


func (NilMeter) Snapshot() Meter { return NilMeter{} }


func (NilMeter) Stop() {}


type StandardMeter struct {
	lock        sync.RWMutex
	snapshot    *MeterSnapshot
	a1, a5, a15 EWMA
	startTime   time.Time
	stopped     uint32
}

func newStandardMeter() *StandardMeter {
	return &StandardMeter{
		snapshot:  &MeterSnapshot{},
		a1:        NewEWMA1(),
		a5:        NewEWMA5(),
		a15:       NewEWMA15(),
		startTime: time.Now(),
	}
}


func (m *StandardMeter) Stop() {
	stopped := atomic.SwapUint32(&m.stopped, 1)
	if stopped != 1 {
		arbiter.Lock()
		delete(arbiter.meters, m)
		arbiter.Unlock()
	}
}



func (m *StandardMeter) Count() int64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateMeter()
	return m.snapshot.count
}


func (m *StandardMeter) Mark(n int64) {
	atomic.AddInt64(&m.snapshot.temp, n)
}


func (m *StandardMeter) Rate1() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snapshot.rate1
}


func (m *StandardMeter) Rate5() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snapshot.rate5
}


func (m *StandardMeter) Rate15() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snapshot.rate15
}


func (m *StandardMeter) RateMean() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.snapshot.rateMean
}


func (m *StandardMeter) Snapshot() Meter {
	m.lock.RLock()
	snapshot := *m.snapshot
	m.lock.RUnlock()
	return &snapshot
}

func (m *StandardMeter) updateSnapshot() {
	
	snapshot := m.snapshot
	snapshot.rate1 = m.a1.Rate()
	snapshot.rate5 = m.a5.Rate()
	snapshot.rate15 = m.a15.Rate()
	snapshot.rateMean = float64(snapshot.count) / time.Since(m.startTime).Seconds()
}

func (m *StandardMeter) updateMeter() {
	
	n := atomic.SwapInt64(&m.snapshot.temp, 0)
	m.snapshot.count += n
	m.a1.Update(n)
	m.a5.Update(n)
	m.a15.Update(n)
}

func (m *StandardMeter) tick() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateMeter()
	m.a1.Tick()
	m.a5.Tick()
	m.a15.Tick()
	m.updateSnapshot()
}



type meterArbiter struct {
	sync.RWMutex
	started bool
	meters  map[*StandardMeter]struct{}
	ticker  *time.Ticker
}

var arbiter = meterArbiter{ticker: time.NewTicker(5 * time.Second), meters: make(map[*StandardMeter]struct{})}


func (ma *meterArbiter) tick() {
	for range ma.ticker.C {
		ma.tickMeters()
	}
}

func (ma *meterArbiter) tickMeters() {
	ma.RLock()
	defer ma.RUnlock()
	for meter := range ma.meters {
		meter.tick()
	}
}

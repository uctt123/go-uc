
















package mclock

import (
	"time"

	"github.com/aristanetworks/goarista/monotime"
)


type AbsTime time.Duration


func Now() AbsTime {
	return AbsTime(monotime.Now())
}


func (t AbsTime) Add(d time.Duration) AbsTime {
	return t + AbsTime(d)
}


func (t AbsTime) Sub(t2 AbsTime) time.Duration {
	return time.Duration(t - t2)
}



type Clock interface {
	Now() AbsTime
	Sleep(time.Duration)
	NewTimer(time.Duration) ChanTimer
	After(time.Duration) <-chan AbsTime
	AfterFunc(d time.Duration, f func()) Timer
}


type Timer interface {
	
	
	Stop() bool
}


type ChanTimer interface {
	Timer

	
	C() <-chan AbsTime
	
	
	Reset(time.Duration)
}


type System struct{}


func (c System) Now() AbsTime {
	return AbsTime(monotime.Now())
}


func (c System) Sleep(d time.Duration) {
	time.Sleep(d)
}


func (c System) NewTimer(d time.Duration) ChanTimer {
	ch := make(chan AbsTime, 1)
	t := time.AfterFunc(d, func() {
		
		
		
		select {
		case ch <- c.Now():
		default:
		}
	})
	return &systemTimer{t, ch}
}


func (c System) After(d time.Duration) <-chan AbsTime {
	ch := make(chan AbsTime, 1)
	time.AfterFunc(d, func() { ch <- c.Now() })
	return ch
}


func (c System) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

type systemTimer struct {
	*time.Timer
	ch <-chan AbsTime
}

func (st *systemTimer) Reset(d time.Duration) {
	st.Timer.Reset(d)
}

func (st *systemTimer) C() <-chan AbsTime {
	return st.ch
}

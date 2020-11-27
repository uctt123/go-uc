















package flowcontrol

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
)



type logger struct {
	events           map[uint64]logEvent
	writePtr, delPtr uint64
	keep             time.Duration
}


type logEvent struct {
	time  mclock.AbsTime
	event string
}


func newLogger(keep time.Duration) *logger {
	return &logger{
		events: make(map[uint64]logEvent),
		keep:   keep,
	}
}


func (l *logger) add(now mclock.AbsTime, event string) {
	keepAfter := now - mclock.AbsTime(l.keep)
	for l.delPtr < l.writePtr && l.events[l.delPtr].time <= keepAfter {
		delete(l.events, l.delPtr)
		l.delPtr++
	}
	l.events[l.writePtr] = logEvent{now, event}
	l.writePtr++
}


func (l *logger) dump(now mclock.AbsTime) {
	for i := l.delPtr; i < l.writePtr; i++ {
		e := l.events[i]
		fmt.Println(time.Duration(e.time-now), e.event)
	}
}

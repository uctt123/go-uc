















package utils

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
)

type UpdateTimer struct {
	clock     mclock.Clock
	lock      sync.Mutex
	last      mclock.AbsTime
	threshold time.Duration
}

func NewUpdateTimer(clock mclock.Clock, threshold time.Duration) *UpdateTimer {
	
	if threshold < 0 {
		return nil
	}
	
	if clock == nil {
		clock = mclock.System{}
	}
	return &UpdateTimer{
		clock:     clock,
		last:      clock.Now(),
		threshold: threshold,
	}
}

func (t *UpdateTimer) Update(callback func(diff time.Duration) bool) bool {
	return t.UpdateAt(t.clock.Now(), callback)
}

func (t *UpdateTimer) UpdateAt(at mclock.AbsTime, callback func(diff time.Duration) bool) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	diff := time.Duration(at - t.last)
	if diff < 0 {
		diff = 0
	}
	if diff < t.threshold {
		return false
	}
	if callback(diff) {
		t.last = at
		return true
	}
	return false
}

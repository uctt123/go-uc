
















package testlog

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/log"
)


func Handler(t *testing.T, level log.Lvl) log.Handler {
	return log.LvlFilterHandler(level, &handler{t, log.TerminalFormat(false)})
}

type handler struct {
	t   *testing.T
	fmt log.Format
}

func (h *handler) Log(r *log.Record) error {
	h.t.Logf("%s", h.fmt.Format(r))
	return nil
}





type logger struct {
	t  *testing.T
	l  log.Logger
	mu *sync.Mutex
	h  *bufHandler
}

type bufHandler struct {
	buf []*log.Record
	fmt log.Format
}

func (h *bufHandler) Log(r *log.Record) error {
	h.buf = append(h.buf, r)
	return nil
}


func Logger(t *testing.T, level log.Lvl) log.Logger {
	l := &logger{
		t:  t,
		l:  log.New(),
		mu: new(sync.Mutex),
		h:  &bufHandler{fmt: log.TerminalFormat(false)},
	}
	l.l.SetHandler(log.LvlFilterHandler(level, l.h))
	return l
}

func (l *logger) Trace(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Trace(msg, ctx...)
	l.flush()
}

func (l *logger) Debug(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Debug(msg, ctx...)
	l.flush()
}

func (l *logger) Info(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Info(msg, ctx...)
	l.flush()
}

func (l *logger) Warn(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Warn(msg, ctx...)
	l.flush()
}

func (l *logger) Error(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Error(msg, ctx...)
	l.flush()
}

func (l *logger) Crit(msg string, ctx ...interface{}) {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Crit(msg, ctx...)
	l.flush()
}

func (l *logger) New(ctx ...interface{}) log.Logger {
	return &logger{l.t, l.l.New(ctx...), l.mu, l.h}
}

func (l *logger) GetHandler() log.Handler {
	return l.l.GetHandler()
}

func (l *logger) SetHandler(h log.Handler) {
	l.l.SetHandler(h)
}


func (l *logger) flush() {
	l.t.Helper()
	for _, r := range l.h.buf {
		l.t.Logf("%s", l.h.fmt.Format(r))
	}
	l.h.buf = nil
}

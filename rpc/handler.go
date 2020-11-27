















package rpc

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
)






















type handler struct {
	reg            *serviceRegistry
	unsubscribeCb  *callback
	idgen          func() ID                      
	respWait       map[string]*requestOp          
	clientSubs     map[string]*ClientSubscription 
	callWG         sync.WaitGroup                 
	rootCtx        context.Context                
	cancelRoot     func()                         
	conn           jsonWriter                     
	log            log.Logger
	allowSubscribe bool

	subLock    sync.Mutex
	serverSubs map[ID]*Subscription
}

type callProc struct {
	ctx       context.Context
	notifiers []*Notifier
}

func newHandler(connCtx context.Context, conn jsonWriter, idgen func() ID, reg *serviceRegistry) *handler {
	rootCtx, cancelRoot := context.WithCancel(connCtx)
	h := &handler{
		reg:            reg,
		idgen:          idgen,
		conn:           conn,
		respWait:       make(map[string]*requestOp),
		clientSubs:     make(map[string]*ClientSubscription),
		rootCtx:        rootCtx,
		cancelRoot:     cancelRoot,
		allowSubscribe: true,
		serverSubs:     make(map[ID]*Subscription),
		log:            log.Root(),
	}
	if conn.remoteAddr() != "" {
		h.log = h.log.New("conn", conn.remoteAddr())
	}
	h.unsubscribeCb = newCallback(reflect.Value{}, reflect.ValueOf(h.unsubscribe))
	return h
}


func (h *handler) handleBatch(msgs []*jsonrpcMessage) {
	
	if len(msgs) == 0 {
		h.startCallProc(func(cp *callProc) {
			h.conn.writeJSON(cp.ctx, errorMessage(&invalidRequestError{"empty batch"}))
		})
		return
	}

	
	calls := make([]*jsonrpcMessage, 0, len(msgs))
	for _, msg := range msgs {
		if handled := h.handleImmediate(msg); !handled {
			calls = append(calls, msg)
		}
	}
	if len(calls) == 0 {
		return
	}
	
	h.startCallProc(func(cp *callProc) {
		answers := make([]*jsonrpcMessage, 0, len(msgs))
		for _, msg := range calls {
			if answer := h.handleCallMsg(cp, msg); answer != nil {
				answers = append(answers, answer)
			}
		}
		h.addSubscriptions(cp.notifiers)
		if len(answers) > 0 {
			h.conn.writeJSON(cp.ctx, answers)
		}
		for _, n := range cp.notifiers {
			n.activate()
		}
	})
}


func (h *handler) handleMsg(msg *jsonrpcMessage) {
	if ok := h.handleImmediate(msg); ok {
		return
	}
	h.startCallProc(func(cp *callProc) {
		answer := h.handleCallMsg(cp, msg)
		h.addSubscriptions(cp.notifiers)
		if answer != nil {
			h.conn.writeJSON(cp.ctx, answer)
		}
		for _, n := range cp.notifiers {
			n.activate()
		}
	})
}



func (h *handler) close(err error, inflightReq *requestOp) {
	h.cancelAllRequests(err, inflightReq)
	h.callWG.Wait()
	h.cancelRoot()
	h.cancelServerSubscriptions(err)
}


func (h *handler) addRequestOp(op *requestOp) {
	for _, id := range op.ids {
		h.respWait[string(id)] = op
	}
}


func (h *handler) removeRequestOp(op *requestOp) {
	for _, id := range op.ids {
		delete(h.respWait, string(id))
	}
}


func (h *handler) cancelAllRequests(err error, inflightReq *requestOp) {
	didClose := make(map[*requestOp]bool)
	if inflightReq != nil {
		didClose[inflightReq] = true
	}

	for id, op := range h.respWait {
		
		delete(h.respWait, id)

		if !didClose[op] {
			op.err = err
			close(op.resp)
			didClose[op] = true
		}
	}
	for id, sub := range h.clientSubs {
		delete(h.clientSubs, id)
		sub.quitWithError(false, err)
	}
}

func (h *handler) addSubscriptions(nn []*Notifier) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	for _, n := range nn {
		if sub := n.takeSubscription(); sub != nil {
			h.serverSubs[sub.ID] = sub
		}
	}
}


func (h *handler) cancelServerSubscriptions(err error) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	for id, s := range h.serverSubs {
		s.err <- err
		close(s.err)
		delete(h.serverSubs, id)
	}
}


func (h *handler) startCallProc(fn func(*callProc)) {
	h.callWG.Add(1)
	go func() {
		ctx, cancel := context.WithCancel(h.rootCtx)
		defer h.callWG.Done()
		defer cancel()
		fn(&callProc{ctx: ctx})
	}()
}



func (h *handler) handleImmediate(msg *jsonrpcMessage) bool {
	start := time.Now()
	switch {
	case msg.isNotification():
		if strings.HasSuffix(msg.Method, notificationMethodSuffix) {
			h.handleSubscriptionResult(msg)
			return true
		}
		return false
	case msg.isResponse():
		h.handleResponse(msg)
		h.log.Trace("Handled RPC response", "reqid", idForLog{msg.ID}, "t", time.Since(start))
		return true
	default:
		return false
	}
}


func (h *handler) handleSubscriptionResult(msg *jsonrpcMessage) {
	var result subscriptionResult
	if err := json.Unmarshal(msg.Params, &result); err != nil {
		h.log.Debug("Dropping invalid subscription message")
		return
	}
	if h.clientSubs[result.ID] != nil {
		h.clientSubs[result.ID].deliver(result.Result)
	}
}


func (h *handler) handleResponse(msg *jsonrpcMessage) {
	op := h.respWait[string(msg.ID)]
	if op == nil {
		h.log.Debug("Unsolicited RPC response", "reqid", idForLog{msg.ID})
		return
	}
	delete(h.respWait, string(msg.ID))
	
	if op.sub == nil {
		op.resp <- msg
		return
	}
	
	
	
	defer close(op.resp)
	if msg.Error != nil {
		op.err = msg.Error
		return
	}
	if op.err = json.Unmarshal(msg.Result, &op.sub.subid); op.err == nil {
		go op.sub.start()
		h.clientSubs[op.sub.subid] = op.sub
	}
}


func (h *handler) handleCallMsg(ctx *callProc, msg *jsonrpcMessage) *jsonrpcMessage {
	start := time.Now()
	switch {
	case msg.isNotification():
		h.handleCall(ctx, msg)
		h.log.Debug("Served "+msg.Method, "t", time.Since(start))
		return nil
	case msg.isCall():
		resp := h.handleCall(ctx, msg)
		var ctx []interface{}
		ctx = append(ctx, "reqid", idForLog{msg.ID}, "t", time.Since(start))
		if resp.Error != nil {
			ctx = append(ctx, "err", resp.Error.Message)
			if resp.Error.Data != nil {
				ctx = append(ctx, "errdata", resp.Error.Data)
			}
			h.log.Warn("Served "+msg.Method, ctx...)
		} else {
			h.log.Debug("Served "+msg.Method, ctx...)
		}
		return resp
	case msg.hasValidID():
		return msg.errorResponse(&invalidRequestError{"invalid request"})
	default:
		return errorMessage(&invalidRequestError{"invalid request"})
	}
}


func (h *handler) handleCall(cp *callProc, msg *jsonrpcMessage) *jsonrpcMessage {
	if msg.isSubscribe() {
		return h.handleSubscribe(cp, msg)
	}
	var callb *callback
	if msg.isUnsubscribe() {
		callb = h.unsubscribeCb
	} else {
		callb = h.reg.callback(msg.Method)
	}
	if callb == nil {
		return msg.errorResponse(&methodNotFoundError{method: msg.Method})
	}
	args, err := parsePositionalArguments(msg.Params, callb.argTypes)
	if err != nil {
		return msg.errorResponse(&invalidParamsError{err.Error()})
	}
	start := time.Now()
	answer := h.runMethod(cp.ctx, msg, callb, args)

	
	
	if callb != h.unsubscribeCb {
		rpcRequestGauge.Inc(1)
		if answer.Error != nil {
			failedReqeustGauge.Inc(1)
		} else {
			successfulRequestGauge.Inc(1)
		}
		rpcServingTimer.UpdateSince(start)
		newRPCServingTimer(msg.Method, answer.Error == nil).UpdateSince(start)
	}
	return answer
}


func (h *handler) handleSubscribe(cp *callProc, msg *jsonrpcMessage) *jsonrpcMessage {
	if !h.allowSubscribe {
		return msg.errorResponse(ErrNotificationsUnsupported)
	}

	
	name, err := parseSubscriptionName(msg.Params)
	if err != nil {
		return msg.errorResponse(&invalidParamsError{err.Error()})
	}
	namespace := msg.namespace()
	callb := h.reg.subscription(namespace, name)
	if callb == nil {
		return msg.errorResponse(&subscriptionNotFoundError{namespace, name})
	}

	
	argTypes := append([]reflect.Type{stringType}, callb.argTypes...)
	args, err := parsePositionalArguments(msg.Params, argTypes)
	if err != nil {
		return msg.errorResponse(&invalidParamsError{err.Error()})
	}
	args = args[1:]

	
	n := &Notifier{h: h, namespace: namespace}
	cp.notifiers = append(cp.notifiers, n)
	ctx := context.WithValue(cp.ctx, notifierKey{}, n)

	return h.runMethod(ctx, msg, callb, args)
}


func (h *handler) runMethod(ctx context.Context, msg *jsonrpcMessage, callb *callback, args []reflect.Value) *jsonrpcMessage {
	result, err := callb.call(ctx, msg.Method, args)
	if err != nil {
		return msg.errorResponse(err)
	}
	return msg.response(result)
}


func (h *handler) unsubscribe(ctx context.Context, id ID) (bool, error) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	s := h.serverSubs[id]
	if s == nil {
		return false, ErrSubscriptionNotFound
	}
	close(s.err)
	delete(h.serverSubs, id)
	return true, nil
}

type idForLog struct{ json.RawMessage }

func (id idForLog) String() string {
	if s, err := strconv.Unquote(string(id.RawMessage)); err == nil {
		return s
	}
	return string(id.RawMessage)
}

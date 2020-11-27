















package rpc

import (
	"container/list"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"time"
)

var (
	
	ErrNotificationsUnsupported = errors.New("notifications not supported")
	
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

var globalGen = randomIDGenerator()


type ID string


func NewID() ID {
	return globalGen()
}


func randomIDGenerator() func() ID {
	var buf = make([]byte, 8)
	var seed int64
	if _, err := crand.Read(buf); err == nil {
		seed = int64(binary.BigEndian.Uint64(buf))
	} else {
		seed = int64(time.Now().Nanosecond())
	}

	var (
		mu  sync.Mutex
		rng = rand.New(rand.NewSource(seed))
	)
	return func() ID {
		mu.Lock()
		defer mu.Unlock()
		id := make([]byte, 16)
		rng.Read(id)
		return encodeID(id)
	}
}

func encodeID(b []byte) ID {
	id := hex.EncodeToString(b)
	id = strings.TrimLeft(id, "0")
	if id == "" {
		id = "0" 
	}
	return ID("0x" + id)
}

type notifierKey struct{}


func NotifierFromContext(ctx context.Context) (*Notifier, bool) {
	n, ok := ctx.Value(notifierKey{}).(*Notifier)
	return n, ok
}



type Notifier struct {
	h         *handler
	namespace string

	mu           sync.Mutex
	sub          *Subscription
	buffer       []json.RawMessage
	callReturned bool
	activated    bool
}





func (n *Notifier) CreateSubscription() *Subscription {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.sub != nil {
		panic("can't create multiple subscriptions with Notifier")
	} else if n.callReturned {
		panic("can't create subscription after subscribe call has returned")
	}
	n.sub = &Subscription{ID: n.h.idgen(), namespace: n.namespace, err: make(chan error, 1)}
	return n.sub
}



func (n *Notifier) Notify(id ID, data interface{}) error {
	enc, err := json.Marshal(data)
	if err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.sub == nil {
		panic("can't Notify before subscription is created")
	} else if n.sub.ID != id {
		panic("Notify with wrong ID")
	}
	if n.activated {
		return n.send(n.sub, enc)
	}
	n.buffer = append(n.buffer, enc)
	return nil
}



func (n *Notifier) Closed() <-chan interface{} {
	return n.h.conn.closed()
}



func (n *Notifier) takeSubscription() *Subscription {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callReturned = true
	return n.sub
}




func (n *Notifier) activate() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, data := range n.buffer {
		if err := n.send(n.sub, data); err != nil {
			return err
		}
	}
	n.activated = true
	return nil
}

func (n *Notifier) send(sub *Subscription, data json.RawMessage) error {
	params, _ := json.Marshal(&subscriptionResult{ID: string(sub.ID), Result: data})
	ctx := context.Background()
	return n.h.conn.writeJSON(ctx, &jsonrpcMessage{
		Version: vsn,
		Method:  n.namespace + notificationMethodSuffix,
		Params:  params,
	})
}



type Subscription struct {
	ID        ID
	namespace string
	err       chan error 
}


func (s *Subscription) Err() <-chan error {
	return s.err
}


func (s *Subscription) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ID)
}



type ClientSubscription struct {
	client    *Client
	etype     reflect.Type
	channel   reflect.Value
	namespace string
	subid     string
	in        chan json.RawMessage

	quitOnce sync.Once     
	quit     chan struct{} 
	errOnce  sync.Once     
	err      chan error
}

func newClientSubscription(c *Client, namespace string, channel reflect.Value) *ClientSubscription {
	sub := &ClientSubscription{
		client:    c,
		namespace: namespace,
		etype:     channel.Type().Elem(),
		channel:   channel,
		quit:      make(chan struct{}),
		err:       make(chan error, 1),
		in:        make(chan json.RawMessage),
	}
	return sub
}









func (sub *ClientSubscription) Err() <-chan error {
	return sub.err
}



func (sub *ClientSubscription) Unsubscribe() {
	sub.quitWithError(true, nil)
	sub.errOnce.Do(func() { close(sub.err) })
}

func (sub *ClientSubscription) quitWithError(unsubscribeServer bool, err error) {
	sub.quitOnce.Do(func() {
		
		
		
		close(sub.quit)
		if unsubscribeServer {
			sub.requestUnsubscribe()
		}
		if err != nil {
			if err == ErrClientQuit {
				err = nil 
			}
			sub.err <- err
		}
	})
}

func (sub *ClientSubscription) deliver(result json.RawMessage) (ok bool) {
	select {
	case sub.in <- result:
		return true
	case <-sub.quit:
		return false
	}
}

func (sub *ClientSubscription) start() {
	sub.quitWithError(sub.forward())
}

func (sub *ClientSubscription) forward() (unsubscribeServer bool, err error) {
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.quit)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.in)},
		{Dir: reflect.SelectSend, Chan: sub.channel},
	}
	buffer := list.New()
	defer buffer.Init()
	for {
		var chosen int
		var recv reflect.Value
		if buffer.Len() == 0 {
			
			chosen, recv, _ = reflect.Select(cases[:2])
		} else {
			
			cases[2].Send = reflect.ValueOf(buffer.Front().Value)
			chosen, recv, _ = reflect.Select(cases)
		}

		switch chosen {
		case 0: 
			return false, nil
		case 1: 
			val, err := sub.unmarshal(recv.Interface().(json.RawMessage))
			if err != nil {
				return true, err
			}
			if buffer.Len() == maxClientSubscriptionBuffer {
				return true, ErrSubscriptionQueueOverflow
			}
			buffer.PushBack(val)
		case 2: 
			cases[2].Send = reflect.Value{} 
			buffer.Remove(buffer.Front())
		}
	}
}

func (sub *ClientSubscription) unmarshal(result json.RawMessage) (interface{}, error) {
	val := reflect.New(sub.etype)
	err := json.Unmarshal(result, val.Interface())
	return val.Elem().Interface(), err
}

func (sub *ClientSubscription) requestUnsubscribe() error {
	var result interface{}
	return sub.client.Call(&result, sub.namespace+unsubscribeMethodSuffix, sub.subid)
}

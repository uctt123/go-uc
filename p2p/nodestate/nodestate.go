















package nodestate

import (
	"errors"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidField = errors.New("invalid field type")
	ErrClosed       = errors.New("already closed")
)

type (
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	NodeStateMachine struct {
		started, closed     bool
		lock                sync.Mutex
		clock               mclock.Clock
		db                  ethdb.KeyValueStore
		dbNodeKey           []byte
		nodes               map[enode.ID]*nodeInfo
		offlineCallbackList []offlineCallback
		opFlag              bool       
		opWait              *sync.Cond 
		opPending           []func()   

		
		
		setup     *Setup
		fields    []*fieldInfo
		saveFlags bitMask

		
		
		stateSubs []stateSub

		
		saveNodeHook func(*nodeInfo)
	}

	
	Flags struct {
		mask  bitMask
		setup *Setup
	}

	
	Field struct {
		index int
		setup *Setup
	}

	
	
	
	flagDefinition struct {
		name       string
		persistent bool
	}

	
	
	
	fieldDefinition struct {
		name   string
		ftype  reflect.Type
		encode func(interface{}) ([]byte, error)
		decode func([]byte) (interface{}, error)
	}

	
	Setup struct {
		Version uint
		flags   []flagDefinition
		fields  []fieldDefinition
	}

	
	
	bitMask uint64

	
	
	
	
	StateCallback func(n *enode.Node, oldState, newState Flags)

	
	
	FieldCallback func(n *enode.Node, state Flags, oldValue, newValue interface{})

	
	nodeInfo struct {
		node       *enode.Node
		state      bitMask
		timeouts   []*nodeStateTimeout
		fields     []interface{}
		fieldCount int
		db, dirty  bool
	}

	nodeInfoEnc struct {
		Enr     enr.Record
		Version uint
		State   bitMask
		Fields  [][]byte
	}

	stateSub struct {
		mask     bitMask
		callback StateCallback
	}

	nodeStateTimeout struct {
		mask  bitMask
		timer mclock.Timer
	}

	fieldInfo struct {
		fieldDefinition
		subs []FieldCallback
	}

	offlineCallback struct {
		node   *nodeInfo
		state  bitMask
		fields []interface{}
	}
)



const offlineState = bitMask(1)


func (s *Setup) NewFlag(name string) Flags {
	if s.flags == nil {
		s.flags = []flagDefinition{{name: "offline"}}
	}
	f := Flags{mask: bitMask(1) << uint(len(s.flags)), setup: s}
	s.flags = append(s.flags, flagDefinition{name: name})
	return f
}


func (s *Setup) NewPersistentFlag(name string) Flags {
	if s.flags == nil {
		s.flags = []flagDefinition{{name: "offline"}}
	}
	f := Flags{mask: bitMask(1) << uint(len(s.flags)), setup: s}
	s.flags = append(s.flags, flagDefinition{name: name, persistent: true})
	return f
}


func (s *Setup) OfflineFlag() Flags {
	return Flags{mask: offlineState, setup: s}
}


func (s *Setup) NewField(name string, ftype reflect.Type) Field {
	f := Field{index: len(s.fields), setup: s}
	s.fields = append(s.fields, fieldDefinition{
		name:  name,
		ftype: ftype,
	})
	return f
}


func (s *Setup) NewPersistentField(name string, ftype reflect.Type, encode func(interface{}) ([]byte, error), decode func([]byte) (interface{}, error)) Field {
	f := Field{index: len(s.fields), setup: s}
	s.fields = append(s.fields, fieldDefinition{
		name:   name,
		ftype:  ftype,
		encode: encode,
		decode: decode,
	})
	return f
}


func flagOp(a, b Flags, trueIfA, trueIfB, trueIfBoth bool) Flags {
	if a.setup == nil {
		if a.mask != 0 {
			panic("Node state flags have no setup reference")
		}
		a.setup = b.setup
	}
	if b.setup == nil {
		if b.mask != 0 {
			panic("Node state flags have no setup reference")
		}
		b.setup = a.setup
	}
	if a.setup != b.setup {
		panic("Node state flags belong to a different setup")
	}
	res := Flags{setup: a.setup}
	if trueIfA {
		res.mask |= a.mask & ^b.mask
	}
	if trueIfB {
		res.mask |= b.mask & ^a.mask
	}
	if trueIfBoth {
		res.mask |= a.mask & b.mask
	}
	return res
}


func (a Flags) And(b Flags) Flags { return flagOp(a, b, false, false, true) }


func (a Flags) AndNot(b Flags) Flags { return flagOp(a, b, true, false, false) }


func (a Flags) Or(b Flags) Flags { return flagOp(a, b, true, true, true) }


func (a Flags) Xor(b Flags) Flags { return flagOp(a, b, true, true, false) }


func (a Flags) HasAll(b Flags) bool { return flagOp(a, b, false, true, false).mask == 0 }


func (a Flags) HasNone(b Flags) bool { return flagOp(a, b, false, false, true).mask == 0 }


func (a Flags) Equals(b Flags) bool { return flagOp(a, b, true, true, false).mask == 0 }


func (a Flags) IsEmpty() bool { return a.mask == 0 }


func MergeFlags(list ...Flags) Flags {
	if len(list) == 0 {
		return Flags{}
	}
	res := list[0]
	for i := 1; i < len(list); i++ {
		res = res.Or(list[i])
	}
	return res
}


func (f Flags) String() string {
	if f.mask == 0 {
		return "[]"
	}
	s := "["
	comma := false
	for index, flag := range f.setup.flags {
		if f.mask&(bitMask(1)<<uint(index)) != 0 {
			if comma {
				s = s + ", "
			}
			s = s + flag.name
			comma = true
		}
	}
	s = s + "]"
	return s
}




func NewNodeStateMachine(db ethdb.KeyValueStore, dbKey []byte, clock mclock.Clock, setup *Setup) *NodeStateMachine {
	if setup.flags == nil {
		panic("No state flags defined")
	}
	if len(setup.flags) > 8*int(unsafe.Sizeof(bitMask(0))) {
		panic("Too many node state flags")
	}
	ns := &NodeStateMachine{
		db:        db,
		dbNodeKey: dbKey,
		clock:     clock,
		setup:     setup,
		nodes:     make(map[enode.ID]*nodeInfo),
		fields:    make([]*fieldInfo, len(setup.fields)),
	}
	ns.opWait = sync.NewCond(&ns.lock)
	stateNameMap := make(map[string]int)
	for index, flag := range setup.flags {
		if _, ok := stateNameMap[flag.name]; ok {
			panic("Node state flag name collision: " + flag.name)
		}
		stateNameMap[flag.name] = index
		if flag.persistent {
			ns.saveFlags |= bitMask(1) << uint(index)
		}
	}
	fieldNameMap := make(map[string]int)
	for index, field := range setup.fields {
		if _, ok := fieldNameMap[field.name]; ok {
			panic("Node field name collision: " + field.name)
		}
		ns.fields[index] = &fieldInfo{fieldDefinition: field}
		fieldNameMap[field.name] = index
	}
	return ns
}


func (ns *NodeStateMachine) stateMask(flags Flags) bitMask {
	if flags.setup != ns.setup && flags.mask != 0 {
		panic("Node state flags belong to a different setup")
	}
	return flags.mask
}


func (ns *NodeStateMachine) fieldIndex(field Field) int {
	if field.setup != ns.setup {
		panic("Node field belongs to a different setup")
	}
	return field.index
}










func (ns *NodeStateMachine) SubscribeState(flags Flags, callback StateCallback) {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	if ns.started {
		panic("state machine already started")
	}
	ns.stateSubs = append(ns.stateSubs, stateSub{ns.stateMask(flags), callback})
}


func (ns *NodeStateMachine) SubscribeField(field Field, callback FieldCallback) {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	if ns.started {
		panic("state machine already started")
	}
	f := ns.fields[ns.fieldIndex(field)]
	f.subs = append(f.subs, callback)
}


func (ns *NodeStateMachine) newNode(n *enode.Node) *nodeInfo {
	return &nodeInfo{node: n, fields: make([]interface{}, len(ns.fields))}
}


func (ns *NodeStateMachine) checkStarted() {
	if !ns.started {
		panic("state machine not started yet")
	}
}



func (ns *NodeStateMachine) Start() {
	ns.lock.Lock()
	if ns.started {
		panic("state machine already started")
	}
	ns.started = true
	if ns.db != nil {
		ns.loadFromDb()
	}

	ns.opStart()
	ns.offlineCallbacks(true)
	ns.opFinish()
	ns.lock.Unlock()
}


func (ns *NodeStateMachine) Stop() {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.checkStarted()
	if !ns.opStart() {
		panic("already closed")
	}
	for _, node := range ns.nodes {
		fields := make([]interface{}, len(node.fields))
		copy(fields, node.fields)
		ns.offlineCallbackList = append(ns.offlineCallbackList, offlineCallback{node, node.state, fields})
	}
	if ns.db != nil {
		ns.saveToDb()
	}
	ns.offlineCallbacks(false)
	ns.closed = true
	ns.opFinish()
}


func (ns *NodeStateMachine) loadFromDb() {
	it := ns.db.NewIterator(ns.dbNodeKey, nil)
	for it.Next() {
		var id enode.ID
		if len(it.Key()) != len(ns.dbNodeKey)+len(id) {
			log.Error("Node state db entry with invalid length", "found", len(it.Key()), "expected", len(ns.dbNodeKey)+len(id))
			continue
		}
		copy(id[:], it.Key()[len(ns.dbNodeKey):])
		ns.decodeNode(id, it.Value())
	}
}

type dummyIdentity enode.ID

func (id dummyIdentity) Verify(r *enr.Record, sig []byte) error { return nil }
func (id dummyIdentity) NodeAddr(r *enr.Record) []byte          { return id[:] }


func (ns *NodeStateMachine) decodeNode(id enode.ID, data []byte) {
	var enc nodeInfoEnc
	if err := rlp.DecodeBytes(data, &enc); err != nil {
		log.Error("Failed to decode node info", "id", id, "error", err)
		return
	}
	n, _ := enode.New(dummyIdentity(id), &enc.Enr)
	node := ns.newNode(n)
	node.db = true

	if enc.Version != ns.setup.Version {
		log.Debug("Removing stored node with unknown version", "current", ns.setup.Version, "stored", enc.Version)
		ns.deleteNode(id)
		return
	}
	if len(enc.Fields) > len(ns.setup.fields) {
		log.Error("Invalid node field count", "id", id, "stored", len(enc.Fields))
		return
	}
	
	for i, encField := range enc.Fields {
		if len(encField) == 0 {
			continue
		}
		if decode := ns.fields[i].decode; decode != nil {
			if field, err := decode(encField); err == nil {
				node.fields[i] = field
				node.fieldCount++
			} else {
				log.Error("Failed to decode node field", "id", id, "field name", ns.fields[i].name, "error", err)
				return
			}
		} else {
			log.Error("Cannot decode node field", "id", id, "field name", ns.fields[i].name)
			return
		}
	}
	
	ns.nodes[id] = node
	node.state = enc.State
	fields := make([]interface{}, len(node.fields))
	copy(fields, node.fields)
	ns.offlineCallbackList = append(ns.offlineCallbackList, offlineCallback{node, node.state, fields})
	log.Debug("Loaded node state", "id", id, "state", Flags{mask: enc.State, setup: ns.setup})
}


func (ns *NodeStateMachine) saveNode(id enode.ID, node *nodeInfo) error {
	if ns.db == nil {
		return nil
	}

	storedState := node.state & ns.saveFlags
	for _, t := range node.timeouts {
		storedState &= ^t.mask
	}
	enc := nodeInfoEnc{
		Enr:     *node.node.Record(),
		Version: ns.setup.Version,
		State:   storedState,
		Fields:  make([][]byte, len(ns.fields)),
	}
	log.Debug("Saved node state", "id", id, "state", Flags{mask: enc.State, setup: ns.setup})
	lastIndex := -1
	for i, f := range node.fields {
		if f == nil {
			continue
		}
		encode := ns.fields[i].encode
		if encode == nil {
			continue
		}
		blob, err := encode(f)
		if err != nil {
			return err
		}
		enc.Fields[i] = blob
		lastIndex = i
	}
	if storedState == 0 && lastIndex == -1 {
		if node.db {
			node.db = false
			ns.deleteNode(id)
		}
		node.dirty = false
		return nil
	}
	enc.Fields = enc.Fields[:lastIndex+1]
	data, err := rlp.EncodeToBytes(&enc)
	if err != nil {
		return err
	}
	if err := ns.db.Put(append(ns.dbNodeKey, id[:]...), data); err != nil {
		return err
	}
	node.dirty, node.db = false, true

	if ns.saveNodeHook != nil {
		ns.saveNodeHook(node)
	}
	return nil
}


func (ns *NodeStateMachine) deleteNode(id enode.ID) {
	ns.db.Delete(append(ns.dbNodeKey, id[:]...))
}


func (ns *NodeStateMachine) saveToDb() {
	for id, node := range ns.nodes {
		if node.dirty {
			err := ns.saveNode(id, node)
			if err != nil {
				log.Error("Failed to save node", "id", id, "error", err)
			}
		}
	}
}


func (ns *NodeStateMachine) updateEnode(n *enode.Node) (enode.ID, *nodeInfo) {
	id := n.ID()
	node := ns.nodes[id]
	if node != nil && n.Seq() > node.node.Seq() {
		node.node = n
	}
	return id, node
}


func (ns *NodeStateMachine) Persist(n *enode.Node) error {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.checkStarted()
	if id, node := ns.updateEnode(n); node != nil && node.dirty {
		err := ns.saveNode(id, node)
		if err != nil {
			log.Error("Failed to save node", "id", id, "error", err)
		}
		return err
	}
	return nil
}



func (ns *NodeStateMachine) SetState(n *enode.Node, setFlags, resetFlags Flags, timeout time.Duration) error {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	if !ns.opStart() {
		return ErrClosed
	}
	ns.setState(n, setFlags, resetFlags, timeout)
	ns.opFinish()
	return nil
}



func (ns *NodeStateMachine) SetStateSub(n *enode.Node, setFlags, resetFlags Flags, timeout time.Duration) {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.opCheck()
	ns.setState(n, setFlags, resetFlags, timeout)
}

func (ns *NodeStateMachine) setState(n *enode.Node, setFlags, resetFlags Flags, timeout time.Duration) {
	ns.checkStarted()
	set, reset := ns.stateMask(setFlags), ns.stateMask(resetFlags)
	id, node := ns.updateEnode(n)
	if node == nil {
		if set == 0 {
			return
		}
		node = ns.newNode(n)
		ns.nodes[id] = node
	}
	oldState := node.state
	newState := (node.state & (^reset)) | set
	changed := oldState ^ newState
	node.state = newState

	
	
	ns.removeTimeouts(node, set|reset)

	
	if timeout != 0 && set != 0 {
		ns.addTimeout(n, set, timeout)
	}
	if newState == oldState {
		return
	}
	if newState == 0 && node.fieldCount == 0 {
		delete(ns.nodes, id)
		if node.db {
			ns.deleteNode(id)
		}
	} else {
		if changed&ns.saveFlags != 0 {
			node.dirty = true
		}
	}
	callback := func() {
		for _, sub := range ns.stateSubs {
			if changed&sub.mask != 0 {
				sub.callback(n, Flags{mask: oldState & sub.mask, setup: ns.setup}, Flags{mask: newState & sub.mask, setup: ns.setup})
			}
		}
	}
	ns.opPending = append(ns.opPending, callback)
}


func (ns *NodeStateMachine) opCheck() {
	if !ns.opFlag {
		panic("Operation has not started")
	}
}


func (ns *NodeStateMachine) opStart() bool {
	for ns.opFlag {
		ns.opWait.Wait()
	}
	if ns.closed {
		return false
	}
	ns.opFlag = true
	return true
}





func (ns *NodeStateMachine) opFinish() {
	for len(ns.opPending) != 0 {
		list := ns.opPending
		ns.lock.Unlock()
		for _, cb := range list {
			cb()
		}
		ns.lock.Lock()
		ns.opPending = ns.opPending[len(list):]
	}
	ns.opPending = nil
	ns.opFlag = false
	ns.opWait.Signal()
}




func (ns *NodeStateMachine) Operation(fn func()) error {
	ns.lock.Lock()
	started := ns.opStart()
	ns.lock.Unlock()
	if !started {
		return ErrClosed
	}
	fn()
	ns.lock.Lock()
	ns.opFinish()
	ns.lock.Unlock()
	return nil
}


func (ns *NodeStateMachine) offlineCallbacks(start bool) {
	for _, cb := range ns.offlineCallbackList {
		cb := cb
		callback := func() {
			for _, sub := range ns.stateSubs {
				offState := offlineState & sub.mask
				onState := cb.state & sub.mask
				if offState == onState {
					continue
				}
				if start {
					sub.callback(cb.node.node, Flags{mask: offState, setup: ns.setup}, Flags{mask: onState, setup: ns.setup})
				} else {
					sub.callback(cb.node.node, Flags{mask: onState, setup: ns.setup}, Flags{mask: offState, setup: ns.setup})
				}
			}
			for i, f := range cb.fields {
				if f == nil || ns.fields[i].subs == nil {
					continue
				}
				for _, fsub := range ns.fields[i].subs {
					if start {
						fsub(cb.node.node, Flags{mask: offlineState, setup: ns.setup}, nil, f)
					} else {
						fsub(cb.node.node, Flags{mask: offlineState, setup: ns.setup}, f, nil)
					}
				}
			}
		}
		ns.opPending = append(ns.opPending, callback)
	}
	ns.offlineCallbackList = nil
}



func (ns *NodeStateMachine) AddTimeout(n *enode.Node, flags Flags, timeout time.Duration) error {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.checkStarted()
	if ns.closed {
		return ErrClosed
	}
	ns.addTimeout(n, ns.stateMask(flags), timeout)
	return nil
}


func (ns *NodeStateMachine) addTimeout(n *enode.Node, mask bitMask, timeout time.Duration) {
	_, node := ns.updateEnode(n)
	if node == nil {
		return
	}
	mask &= node.state
	if mask == 0 {
		return
	}
	ns.removeTimeouts(node, mask)
	t := &nodeStateTimeout{mask: mask}
	t.timer = ns.clock.AfterFunc(timeout, func() {
		ns.SetState(n, Flags{}, Flags{mask: t.mask, setup: ns.setup}, 0)
	})
	node.timeouts = append(node.timeouts, t)
	if mask&ns.saveFlags != 0 {
		node.dirty = true
	}
}





func (ns *NodeStateMachine) removeTimeouts(node *nodeInfo, mask bitMask) {
	for i := 0; i < len(node.timeouts); i++ {
		t := node.timeouts[i]
		match := t.mask & mask
		if match == 0 {
			continue
		}
		t.mask -= match
		if t.mask != 0 {
			continue
		}
		t.timer.Stop()
		node.timeouts[i] = node.timeouts[len(node.timeouts)-1]
		node.timeouts = node.timeouts[:len(node.timeouts)-1]
		i--
		if match&ns.saveFlags != 0 {
			node.dirty = true
		}
	}
}




func (ns *NodeStateMachine) GetField(n *enode.Node, field Field) interface{} {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.checkStarted()
	if ns.closed {
		return nil
	}
	if _, node := ns.updateEnode(n); node != nil {
		return node.fields[ns.fieldIndex(field)]
	}
	return nil
}


func (ns *NodeStateMachine) SetField(n *enode.Node, field Field, value interface{}) error {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	if !ns.opStart() {
		return ErrClosed
	}
	err := ns.setField(n, field, value)
	ns.opFinish()
	return err
}



func (ns *NodeStateMachine) SetFieldSub(n *enode.Node, field Field, value interface{}) error {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.opCheck()
	return ns.setField(n, field, value)
}

func (ns *NodeStateMachine) setField(n *enode.Node, field Field, value interface{}) error {
	ns.checkStarted()
	id, node := ns.updateEnode(n)
	if node == nil {
		if value == nil {
			return nil
		}
		node = ns.newNode(n)
		ns.nodes[id] = node
	}
	fieldIndex := ns.fieldIndex(field)
	f := ns.fields[fieldIndex]
	if value != nil && reflect.TypeOf(value) != f.ftype {
		log.Error("Invalid field type", "type", reflect.TypeOf(value), "required", f.ftype)
		return ErrInvalidField
	}
	oldValue := node.fields[fieldIndex]
	if value == oldValue {
		return nil
	}
	if oldValue != nil {
		node.fieldCount--
	}
	if value != nil {
		node.fieldCount++
	}
	node.fields[fieldIndex] = value
	if node.state == 0 && node.fieldCount == 0 {
		delete(ns.nodes, id)
		if node.db {
			ns.deleteNode(id)
		}
	} else {
		if f.encode != nil {
			node.dirty = true
		}
	}
	state := node.state
	callback := func() {
		for _, cb := range f.subs {
			cb(n, Flags{mask: state, setup: ns.setup}, oldValue, value)
		}
	}
	ns.opPending = append(ns.opPending, callback)
	return nil
}





func (ns *NodeStateMachine) ForEach(requireFlags, disableFlags Flags, cb func(n *enode.Node, state Flags)) {
	ns.lock.Lock()
	ns.checkStarted()
	type callback struct {
		node  *enode.Node
		state bitMask
	}
	require, disable := ns.stateMask(requireFlags), ns.stateMask(disableFlags)
	var callbacks []callback
	for _, node := range ns.nodes {
		if node.state&require == require && node.state&disable == 0 {
			callbacks = append(callbacks, callback{node.node, node.state & (require | disable)})
		}
	}
	ns.lock.Unlock()
	for _, c := range callbacks {
		cb(c.node, Flags{mask: c.state, setup: ns.setup})
	}
}


func (ns *NodeStateMachine) GetNode(id enode.ID) *enode.Node {
	ns.lock.Lock()
	defer ns.lock.Unlock()

	ns.checkStarted()
	if node := ns.nodes[id]; node != nil {
		return node.node
	}
	return nil
}



func (ns *NodeStateMachine) AddLogMetrics(requireFlags, disableFlags Flags, name string, inMeter, outMeter metrics.Meter, gauge metrics.Gauge) {
	var count int64
	ns.SubscribeState(requireFlags.Or(disableFlags), func(n *enode.Node, oldState, newState Flags) {
		oldMatch := oldState.HasAll(requireFlags) && oldState.HasNone(disableFlags)
		newMatch := newState.HasAll(requireFlags) && newState.HasNone(disableFlags)
		if newMatch == oldMatch {
			return
		}

		if newMatch {
			count++
			if name != "" {
				log.Debug("Node entered", "set", name, "id", n.ID(), "count", count)
			}
			if inMeter != nil {
				inMeter.Mark(1)
			}
		} else {
			count--
			if name != "" {
				log.Debug("Node left", "set", name, "id", n.ID(), "count", count)
			}
			if outMeter != nil {
				outMeter.Mark(1)
			}
		}
		if gauge != nil {
			gauge.Update(count)
		}
	})
}

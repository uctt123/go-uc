















package node

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/tsdb/fileutil"
)


type Node struct {
	eventmux      *event.TypeMux
	config        *Config
	accman        *accounts.Manager
	log           log.Logger
	ephemKeystore string            
	dirLock       fileutil.Releaser 
	stop          chan struct{}     
	server        *p2p.Server       
	startStopLock sync.Mutex        
	state         int               

	lock          sync.Mutex
	lifecycles    []Lifecycle 
	rpcAPIs       []rpc.API   
	http          *httpServer 
	ws            *httpServer 
	ipc           *ipcServer  
	inprocHandler *rpc.Server 

	databases map[*closeTrackingDB]struct{} 
}

const (
	initializingState = iota
	runningState
	closedState
)


func New(conf *Config) (*Node, error) {
	
	
	confCopy := *conf
	conf = &confCopy
	if conf.DataDir != "" {
		absdatadir, err := filepath.Abs(conf.DataDir)
		if err != nil {
			return nil, err
		}
		conf.DataDir = absdatadir
	}
	if conf.Logger == nil {
		conf.Logger = log.New()
	}

	
	
	if strings.ContainsAny(conf.Name, `/\`) {
		return nil, errors.New(`Config.Name must not contain '/' or '\'`)
	}
	if conf.Name == datadirDefaultKeyStore {
		return nil, errors.New(`Config.Name cannot be "` + datadirDefaultKeyStore + `"`)
	}
	if strings.HasSuffix(conf.Name, ".ipc") {
		return nil, errors.New(`Config.Name cannot end in ".ipc"`)
	}

	node := &Node{
		config:        conf,
		inprocHandler: rpc.NewServer(),
		eventmux:      new(event.TypeMux),
		log:           conf.Logger,
		stop:          make(chan struct{}),
		server:        &p2p.Server{Config: conf.P2P},
		databases:     make(map[*closeTrackingDB]struct{}),
	}

	
	node.rpcAPIs = append(node.rpcAPIs, node.apis()...)

	
	if err := node.openDataDir(); err != nil {
		return nil, err
	}
	
	
	am, ephemeralKeystore, err := makeAccountManager(conf)
	if err != nil {
		return nil, err
	}
	node.accman = am
	node.ephemKeystore = ephemeralKeystore

	
	node.server.Config.PrivateKey = node.config.NodeKey()
	node.server.Config.Name = node.config.NodeName()
	node.server.Config.Logger = node.log
	if node.server.Config.StaticNodes == nil {
		node.server.Config.StaticNodes = node.config.StaticNodes()
	}
	if node.server.Config.TrustedNodes == nil {
		node.server.Config.TrustedNodes = node.config.TrustedNodes()
	}
	if node.server.Config.NodeDatabase == "" {
		node.server.Config.NodeDatabase = node.config.NodeDB()
	}

	
	node.http = newHTTPServer(node.log, conf.HTTPTimeouts)
	node.ws = newHTTPServer(node.log, rpc.DefaultHTTPTimeouts)
	node.ipc = newIPCServer(node.log, conf.IPCEndpoint())

	return node, nil
}



func (n *Node) Start() error {
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	n.lock.Lock()
	switch n.state {
	case runningState:
		n.lock.Unlock()
		return ErrNodeRunning
	case closedState:
		n.lock.Unlock()
		return ErrNodeStopped
	}
	n.state = runningState
	err := n.startNetworking()
	lifecycles := make([]Lifecycle, len(n.lifecycles))
	copy(lifecycles, n.lifecycles)
	n.lock.Unlock()

	
	if err != nil {
		n.doClose(nil)
		return err
	}
	
	var started []Lifecycle
	for _, lifecycle := range lifecycles {
		if err = lifecycle.Start(); err != nil {
			break
		}
		started = append(started, lifecycle)
	}
	
	if err != nil {
		n.stopServices(started)
		n.doClose(nil)
	}
	return err
}



func (n *Node) Close() error {
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	n.lock.Lock()
	state := n.state
	n.lock.Unlock()
	switch state {
	case initializingState:
		
		return n.doClose(nil)
	case runningState:
		
		var errs []error
		if err := n.stopServices(n.lifecycles); err != nil {
			errs = append(errs, err)
		}
		return n.doClose(errs)
	case closedState:
		return ErrNodeStopped
	default:
		panic(fmt.Sprintf("node is in unknown state %d", state))
	}
}


func (n *Node) doClose(errs []error) error {
	
	
	n.lock.Lock()
	n.state = closedState
	errs = append(errs, n.closeDatabases()...)
	n.lock.Unlock()

	if err := n.accman.Close(); err != nil {
		errs = append(errs, err)
	}
	if n.ephemKeystore != "" {
		if err := os.RemoveAll(n.ephemKeystore); err != nil {
			errs = append(errs, err)
		}
	}

	
	n.closeDataDir()

	
	close(n.stop)

	
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%v", errs)
	}
}


func (n *Node) startNetworking() error {
	n.log.Info("Starting peer-to-peer node", "instance", n.server.Name)
	if err := n.server.Start(); err != nil {
		return convertFileLockError(err)
	}
	err := n.startRPC()
	if err != nil {
		n.stopRPC()
		n.server.Stop()
	}
	return err
}


func containsLifecycle(lfs []Lifecycle, l Lifecycle) bool {
	for _, obj := range lfs {
		if obj == l {
			return true
		}
	}
	return false
}



func (n *Node) stopServices(running []Lifecycle) error {
	n.stopRPC()

	
	failure := &StopError{Services: make(map[reflect.Type]error)}
	for i := len(running) - 1; i >= 0; i-- {
		if err := running[i].Stop(); err != nil {
			failure.Services[reflect.TypeOf(running[i])] = err
		}
	}

	
	n.server.Stop()

	if len(failure.Services) > 0 {
		return failure
	}
	return nil
}

func (n *Node) openDataDir() error {
	if n.config.DataDir == "" {
		return nil 
	}

	instdir := filepath.Join(n.config.DataDir, n.config.name())
	if err := os.MkdirAll(instdir, 0700); err != nil {
		return err
	}
	
	
	release, _, err := fileutil.Flock(filepath.Join(instdir, "LOCK"))
	if err != nil {
		return convertFileLockError(err)
	}
	n.dirLock = release
	return nil
}

func (n *Node) closeDataDir() {
	
	if n.dirLock != nil {
		if err := n.dirLock.Release(); err != nil {
			n.log.Error("Can't release datadir lock", "err", err)
		}
		n.dirLock = nil
	}
}




func (n *Node) startRPC() error {
	if err := n.startInProc(); err != nil {
		return err
	}

	
	if n.ipc.endpoint != "" {
		if err := n.ipc.start(n.rpcAPIs); err != nil {
			return err
		}
	}

	
	if n.config.HTTPHost != "" {
		config := httpConfig{
			CorsAllowedOrigins: n.config.HTTPCors,
			Vhosts:             n.config.HTTPVirtualHosts,
			Modules:            n.config.HTTPModules,
		}
		if err := n.http.setListenAddr(n.config.HTTPHost, n.config.HTTPPort); err != nil {
			return err
		}
		if err := n.http.enableRPC(n.rpcAPIs, config); err != nil {
			return err
		}
	}

	
	if n.config.WSHost != "" {
		server := n.wsServerForPort(n.config.WSPort)
		config := wsConfig{
			Modules: n.config.WSModules,
			Origins: n.config.WSOrigins,
		}
		if err := server.setListenAddr(n.config.WSHost, n.config.WSPort); err != nil {
			return err
		}
		if err := server.enableWS(n.rpcAPIs, config); err != nil {
			return err
		}
	}

	if err := n.http.start(); err != nil {
		return err
	}
	return n.ws.start()
}

func (n *Node) wsServerForPort(port int) *httpServer {
	if n.config.HTTPHost == "" || n.http.port == port {
		return n.http
	}
	return n.ws
}

func (n *Node) stopRPC() {
	n.http.stop()
	n.ws.stop()
	n.ipc.stop()
	n.stopInProc()
}


func (n *Node) startInProc() error {
	for _, api := range n.rpcAPIs {
		if err := n.inprocHandler.RegisterName(api.Namespace, api.Service); err != nil {
			return err
		}
	}
	return nil
}


func (n *Node) stopInProc() {
	n.inprocHandler.Stop()
}


func (n *Node) Wait() {
	<-n.stop
}


func (n *Node) RegisterLifecycle(lifecycle Lifecycle) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register lifecycle on running/stopped node")
	}
	if containsLifecycle(n.lifecycles, lifecycle) {
		panic(fmt.Sprintf("attempt to register lifecycle %T more than once", lifecycle))
	}
	n.lifecycles = append(n.lifecycles, lifecycle)
}


func (n *Node) RegisterProtocols(protocols []p2p.Protocol) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register protocols on running/stopped node")
	}
	n.server.Protocols = append(n.server.Protocols, protocols...)
}


func (n *Node) RegisterAPIs(apis []rpc.API) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register APIs on running/stopped node")
	}
	n.rpcAPIs = append(n.rpcAPIs, apis...)
}





func (n *Node) RegisterHandler(name, path string, handler http.Handler) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register HTTP handler on running/stopped node")
	}
	n.http.mux.Handle(path, handler)
	n.http.handlerNames[path] = name
}


func (n *Node) Attach() (*rpc.Client, error) {
	return rpc.DialInProc(n.inprocHandler), nil
}


func (n *Node) RPCHandler() (*rpc.Server, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state == closedState {
		return nil, ErrNodeStopped
	}
	return n.inprocHandler, nil
}


func (n *Node) Config() *Config {
	return n.config
}




func (n *Node) Server() *p2p.Server {
	n.lock.Lock()
	defer n.lock.Unlock()

	return n.server
}



func (n *Node) DataDir() string {
	return n.config.DataDir
}


func (n *Node) InstanceDir() string {
	return n.config.instanceDir()
}


func (n *Node) AccountManager() *accounts.Manager {
	return n.accman
}


func (n *Node) IPCEndpoint() string {
	return n.ipc.endpoint
}


func (n *Node) HTTPEndpoint() string {
	return "http:
}


func (n *Node) WSEndpoint() string {
	if n.http.wsAllowed() {
		return "ws:
	}
	return "ws:
}



func (n *Node) EventMux() *event.TypeMux {
	return n.eventmux
}




func (n *Node) OpenDatabase(name string, cache, handles int, namespace string) (ethdb.Database, error) {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == closedState {
		return nil, ErrNodeStopped
	}

	var db ethdb.Database
	var err error
	if n.config.DataDir == "" {
		db = rawdb.NewMemoryDatabase()
	} else {
		db, err = rawdb.NewLevelDBDatabase(n.ResolvePath(name), cache, handles, namespace)
	}

	if err == nil {
		db = n.wrapDatabase(db)
	}
	return db, err
}






func (n *Node) OpenDatabaseWithFreezer(name string, cache, handles int, freezer, namespace string) (ethdb.Database, error) {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == closedState {
		return nil, ErrNodeStopped
	}

	var db ethdb.Database
	var err error
	if n.config.DataDir == "" {
		db = rawdb.NewMemoryDatabase()
	} else {
		root := n.ResolvePath(name)
		switch {
		case freezer == "":
			freezer = filepath.Join(root, "ancient")
		case !filepath.IsAbs(freezer):
			freezer = n.ResolvePath(freezer)
		}
		db, err = rawdb.NewLevelDBDatabaseWithFreezer(root, cache, handles, freezer, namespace)
	}

	if err == nil {
		db = n.wrapDatabase(db)
	}
	return db, err
}


func (n *Node) ResolvePath(x string) string {
	return n.config.ResolvePath(x)
}




type closeTrackingDB struct {
	ethdb.Database
	n *Node
}

func (db *closeTrackingDB) Close() error {
	db.n.lock.Lock()
	delete(db.n.databases, db)
	db.n.lock.Unlock()
	return db.Database.Close()
}


func (n *Node) wrapDatabase(db ethdb.Database) ethdb.Database {
	wrapper := &closeTrackingDB{db, n}
	n.databases[wrapper] = struct{}{}
	return wrapper
}


func (n *Node) closeDatabases() (errors []error) {
	for db := range n.databases {
		delete(n.databases, db)
		if err := db.Database.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

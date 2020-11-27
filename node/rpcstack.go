















package node

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/rs/cors"
)


type httpConfig struct {
	Modules            []string
	CorsAllowedOrigins []string
	Vhosts             []string
}


type wsConfig struct {
	Origins []string
	Modules []string
}

type rpcHandler struct {
	http.Handler
	server *rpc.Server
}

type httpServer struct {
	log      log.Logger
	timeouts rpc.HTTPTimeouts
	mux      http.ServeMux 

	mu       sync.Mutex
	server   *http.Server
	listener net.Listener 

	
	httpConfig  httpConfig
	httpHandler atomic.Value 

	
	wsConfig  wsConfig
	wsHandler atomic.Value 

	
	endpoint string
	host     string
	port     int

	handlerNames map[string]string
}

func newHTTPServer(log log.Logger, timeouts rpc.HTTPTimeouts) *httpServer {
	h := &httpServer{log: log, timeouts: timeouts, handlerNames: make(map[string]string)}
	h.httpHandler.Store((*rpcHandler)(nil))
	h.wsHandler.Store((*rpcHandler)(nil))
	return h
}



func (h *httpServer) setListenAddr(host string, port int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.listener != nil && (host != h.host || port != h.port) {
		return fmt.Errorf("HTTP server already running on %s", h.endpoint)
	}

	h.host, h.port = host, port
	h.endpoint = fmt.Sprintf("%s:%d", host, port)
	return nil
}


func (h *httpServer) listenAddr() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.listener != nil {
		return h.listener.Addr().String()
	}
	return h.endpoint
}


func (h *httpServer) start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.endpoint == "" || h.listener != nil {
		return nil 
	}

	
	h.server = &http.Server{Handler: h}
	if h.timeouts != (rpc.HTTPTimeouts{}) {
		CheckTimeouts(&h.timeouts)
		h.server.ReadTimeout = h.timeouts.ReadTimeout
		h.server.WriteTimeout = h.timeouts.WriteTimeout
		h.server.IdleTimeout = h.timeouts.IdleTimeout
	}

	
	listener, err := net.Listen("tcp", h.endpoint)
	if err != nil {
		
		
		h.disableRPC()
		h.disableWS()
		return err
	}
	h.listener = listener
	go h.server.Serve(listener)

	
	if h.wsAllowed() && !h.rpcAllowed() {
		h.log.Info("WebSocket enabled", "url", fmt.Sprintf("ws:
		return nil
	}
	
	h.log.Info("HTTP server started",
		"endpoint", listener.Addr(),
		"cors", strings.Join(h.httpConfig.CorsAllowedOrigins, ","),
		"vhosts", strings.Join(h.httpConfig.Vhosts, ","),
	)

	
	var paths []string
	for path := range h.handlerNames {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	logged := make(map[string]bool, len(paths))
	for _, path := range paths {
		name := h.handlerNames[path]
		if !logged[name] {
			log.Info(name+" enabled", "url", "http:
			logged[name] = true
		}
	}
	return nil
}

func (h *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rpc := h.httpHandler.Load().(*rpcHandler)
	if r.RequestURI == "/" {
		
		ws := h.wsHandler.Load().(*rpcHandler)
		if ws != nil && isWebsocket(r) {
			ws.ServeHTTP(w, r)
			return
		}
		if rpc != nil {
			rpc.ServeHTTP(w, r)
			return
		}
	} else if rpc != nil {
		
		
		
		h.mux.ServeHTTP(w, r)
		return
	}
	w.WriteHeader(404)
}


func (h *httpServer) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.doStop()
}

func (h *httpServer) doStop() {
	if h.listener == nil {
		return 
	}

	
	httpHandler := h.httpHandler.Load().(*rpcHandler)
	wsHandler := h.httpHandler.Load().(*rpcHandler)
	if httpHandler != nil {
		h.httpHandler.Store((*rpcHandler)(nil))
		httpHandler.server.Stop()
	}
	if wsHandler != nil {
		h.wsHandler.Store((*rpcHandler)(nil))
		wsHandler.server.Stop()
	}
	h.server.Shutdown(context.Background())
	h.listener.Close()
	h.log.Info("HTTP server stopped", "endpoint", h.listener.Addr())

	
	h.host, h.port, h.endpoint = "", 0, ""
	h.server, h.listener = nil, nil
}


func (h *httpServer) enableRPC(apis []rpc.API, config httpConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rpcAllowed() {
		return fmt.Errorf("JSON-RPC over HTTP is already enabled")
	}

	
	srv := rpc.NewServer()
	if err := RegisterApisFromWhitelist(apis, config.Modules, srv, false); err != nil {
		return err
	}
	h.httpConfig = config
	h.httpHandler.Store(&rpcHandler{
		Handler: NewHTTPHandlerStack(srv, config.CorsAllowedOrigins, config.Vhosts),
		server:  srv,
	})
	return nil
}


func (h *httpServer) disableRPC() bool {
	handler := h.httpHandler.Load().(*rpcHandler)
	if handler != nil {
		h.httpHandler.Store((*rpcHandler)(nil))
		handler.server.Stop()
	}
	return handler != nil
}


func (h *httpServer) enableWS(apis []rpc.API, config wsConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.wsAllowed() {
		return fmt.Errorf("JSON-RPC over WebSocket is already enabled")
	}

	
	srv := rpc.NewServer()
	if err := RegisterApisFromWhitelist(apis, config.Modules, srv, false); err != nil {
		return err
	}
	h.wsConfig = config
	h.wsHandler.Store(&rpcHandler{
		Handler: srv.WebsocketHandler(config.Origins),
		server:  srv,
	})
	return nil
}


func (h *httpServer) stopWS() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.disableWS() {
		if !h.rpcAllowed() {
			h.doStop()
		}
	}
}


func (h *httpServer) disableWS() bool {
	ws := h.wsHandler.Load().(*rpcHandler)
	if ws != nil {
		h.wsHandler.Store((*rpcHandler)(nil))
		ws.server.Stop()
	}
	return ws != nil
}


func (h *httpServer) rpcAllowed() bool {
	return h.httpHandler.Load().(*rpcHandler) != nil
}


func (h *httpServer) wsAllowed() bool {
	return h.wsHandler.Load().(*rpcHandler) != nil
}


func isWebsocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}


func NewHTTPHandlerStack(srv http.Handler, cors []string, vhosts []string) http.Handler {
	
	handler := newCorsHandler(srv, cors)
	handler = newVHostHandler(vhosts, handler)
	return newGzipHandler(handler)
}

func newCorsHandler(srv http.Handler, allowedOrigins []string) http.Handler {
	
	if len(allowedOrigins) == 0 {
		return srv
	}
	c := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{http.MethodPost, http.MethodGet},
		AllowedHeaders: []string{"*"},
		MaxAge:         600,
	})
	return c.Handler(srv)
}





type virtualHostHandler struct {
	vhosts map[string]struct{}
	next   http.Handler
}

func newVHostHandler(vhosts []string, next http.Handler) http.Handler {
	vhostMap := make(map[string]struct{})
	for _, allowedHost := range vhosts {
		vhostMap[strings.ToLower(allowedHost)] = struct{}{}
	}
	return &virtualHostHandler{vhostMap, next}
}


func (h *virtualHostHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	
	if r.Host == "" {
		h.next.ServeHTTP(w, r)
		return
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		
		host = r.Host
	}
	if ipAddr := net.ParseIP(host); ipAddr != nil {
		
		h.next.ServeHTTP(w, r)
		return

	}
	
	if _, exist := h.vhosts["*"]; exist {
		h.next.ServeHTTP(w, r)
		return
	}
	if _, exist := h.vhosts[host]; exist {
		h.next.ServeHTTP(w, r)
		return
	}
	http.Error(w, "invalid host specified", http.StatusForbidden)
}

var gzPool = sync.Pool{
	New: func() interface{} {
		w := gzip.NewWriter(ioutil.Discard)
		return w
	},
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func newGzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")

		gz := gzPool.Get().(*gzip.Writer)
		defer gzPool.Put(gz)

		gz.Reset(w)
		defer gz.Close()

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

type ipcServer struct {
	log      log.Logger
	endpoint string

	mu       sync.Mutex
	listener net.Listener
	srv      *rpc.Server
}

func newIPCServer(log log.Logger, endpoint string) *ipcServer {
	return &ipcServer{log: log, endpoint: endpoint}
}


func (is *ipcServer) start(apis []rpc.API) error {
	is.mu.Lock()
	defer is.mu.Unlock()

	if is.listener != nil {
		return nil 
	}
	listener, srv, err := rpc.StartIPCEndpoint(is.endpoint, apis)
	if err != nil {
		return err
	}
	is.log.Info("IPC endpoint opened", "url", is.endpoint)
	is.listener, is.srv = listener, srv
	return nil
}

func (is *ipcServer) stop() error {
	is.mu.Lock()
	defer is.mu.Unlock()

	if is.listener == nil {
		return nil 
	}
	err := is.listener.Close()
	is.srv.Stop()
	is.listener, is.srv = nil, nil
	is.log.Info("IPC endpoint closed", "url", is.endpoint)
	return err
}



func RegisterApisFromWhitelist(apis []rpc.API, modules []string, srv *rpc.Server, exposeAll bool) error {
	if bad, available := checkModuleAvailability(modules, apis); len(bad) > 0 {
		log.Error("Unavailable modules in HTTP API list", "unavailable", bad, "available", available)
	}
	
	whitelist := make(map[string]bool)
	for _, module := range modules {
		whitelist[module] = true
	}
	
	for _, api := range apis {
		if exposeAll || whitelist[api.Namespace] || (len(whitelist) == 0 && api.Public) {
			if err := srv.RegisterName(api.Namespace, api.Service); err != nil {
				return err
			}
		}
	}
	return nil
}

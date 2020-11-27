















package rpc

import (
	"context"
	"io"
	"sync/atomic"

	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/log"
)

const MetadataApi = "rpc"




type CodecOption int

const (
	
	OptionMethodInvocation CodecOption = 1 << iota

	
	OptionSubscriptions = 1 << iota 
)


type Server struct {
	services serviceRegistry
	idgen    func() ID
	run      int32
	codecs   mapset.Set
}


func NewServer() *Server {
	server := &Server{idgen: randomIDGenerator(), codecs: mapset.NewSet(), run: 1}
	
	
	rpcService := &RPCService{server}
	server.RegisterName(MetadataApi, rpcService)
	return server
}





func (s *Server) RegisterName(name string, receiver interface{}) error {
	return s.services.registerName(name, receiver)
}






func (s *Server) ServeCodec(codec ServerCodec, options CodecOption) {
	defer codec.close()

	
	if atomic.LoadInt32(&s.run) == 0 {
		return
	}

	
	s.codecs.Add(codec)
	defer s.codecs.Remove(codec)

	c := initClient(codec, s.idgen, &s.services)
	<-codec.closed()
	c.Close()
}




func (s *Server) serveSingleRequest(ctx context.Context, codec ServerCodec) {
	
	if atomic.LoadInt32(&s.run) == 0 {
		return
	}

	h := newHandler(ctx, codec, s.idgen, &s.services)
	h.allowSubscribe = false
	defer h.close(io.EOF, nil)

	reqs, batch, err := codec.readBatch()
	if err != nil {
		if err != io.EOF {
			codec.writeJSON(ctx, errorMessage(&invalidMessageError{"parse error"}))
		}
		return
	}
	if batch {
		h.handleBatch(reqs)
	} else {
		h.handleMsg(reqs[0])
	}
}




func (s *Server) Stop() {
	if atomic.CompareAndSwapInt32(&s.run, 1, 0) {
		log.Debug("RPC server shutting down")
		s.codecs.Each(func(c interface{}) bool {
			c.(ServerCodec).close()
			return true
		})
	}
}



type RPCService struct {
	server *Server
}


func (s *RPCService) Modules() map[string]string {
	s.server.services.mu.Lock()
	defer s.server.services.mu.Unlock()

	modules := make(map[string]string)
	for name := range s.server.services.services {
		modules[name] = "1.0"
	}
	return modules
}

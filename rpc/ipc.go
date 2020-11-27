















package rpc

import (
	"context"
	"net"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)


func (s *Server) ServeListener(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if netutil.IsTemporaryError(err) {
			log.Warn("RPC accept error", "err", err)
			continue
		} else if err != nil {
			return err
		}
		log.Trace("Accepted RPC connection", "conn", conn.RemoteAddr())
		go s.ServeCodec(NewCodec(conn), 0)
	}
}







func DialIPC(ctx context.Context, endpoint string) (*Client, error) {
	return newClient(ctx, func(ctx context.Context) (ServerCodec, error) {
		conn, err := newIPCConnection(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		return NewCodec(conn), err
	})
}

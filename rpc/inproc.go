















package rpc

import (
	"context"
	"net"
)


func DialInProc(handler *Server) *Client {
	initctx := context.Background()
	c, _ := newClient(initctx, func(context.Context) (ServerCodec, error) {
		p1, p2 := net.Pipe()
		go handler.ServeCodec(NewCodec(p1), 0)
		return NewCodec(p2), nil
	})
	return c
}

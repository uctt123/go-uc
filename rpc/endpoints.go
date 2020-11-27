















package rpc

import (
	"net"

	"github.com/ethereum/go-ethereum/log"
)


func StartIPCEndpoint(ipcEndpoint string, apis []API) (net.Listener, *Server, error) {
	
	handler := NewServer()
	for _, api := range apis {
		if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
			return nil, nil, err
		}
		log.Debug("IPC registered", "namespace", api.Namespace)
	}
	
	listener, err := ipcListen(ipcEndpoint)
	if err != nil {
		return nil, nil, err
	}
	go handler.ServeListener(listener)
	return listener, handler, nil
}

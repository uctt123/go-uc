

















package rpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/log"
)


func ipcListen(endpoint string) (net.Listener, error) {
	if len(endpoint) > int(max_path_size) {
		log.Warn(fmt.Sprintf("The ipc endpoint is longer than %d characters. ", max_path_size),
			"endpoint", endpoint)
	}

	
	if err := os.MkdirAll(filepath.Dir(endpoint), 0751); err != nil {
		return nil, err
	}
	os.Remove(endpoint)
	l, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, err
	}
	os.Chmod(endpoint, 0600)
	return l, nil
}


func newIPCConnection(ctx context.Context, endpoint string) (net.Conn, error) {
	return new(net.Dialer).DialContext(ctx, "unix", endpoint)
}

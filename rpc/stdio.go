















package rpc

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"time"
)


func DialStdIO(ctx context.Context) (*Client, error) {
	return DialIO(ctx, os.Stdin, os.Stdout)
}


func DialIO(ctx context.Context, in io.Reader, out io.Writer) (*Client, error) {
	return newClient(ctx, func(_ context.Context) (ServerCodec, error) {
		return NewCodec(stdioConn{
			in:  in,
			out: out,
		}), nil
	})
}

type stdioConn struct {
	in  io.Reader
	out io.Writer
}

func (io stdioConn) Read(b []byte) (n int, err error) {
	return io.in.Read(b)
}

func (io stdioConn) Write(b []byte) (n int, err error) {
	return io.out.Write(b)
}

func (io stdioConn) Close() error {
	return nil
}

func (io stdioConn) RemoteAddr() string {
	return "/dev/stdin"
}

func (io stdioConn) SetWriteDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "stdio", Source: nil, Addr: nil, Err: errors.New("deadline not supported")}
}

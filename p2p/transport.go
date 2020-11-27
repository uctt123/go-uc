















package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/bitutil"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p/rlpx"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	
	
	handshakeTimeout = 5 * time.Second

	
	
	
	discWriteTimeout = 1 * time.Second
)



type rlpxTransport struct {
	rmu, wmu sync.Mutex
	wbuf     bytes.Buffer
	conn     *rlpx.Conn
}

func newRLPX(conn net.Conn, dialDest *ecdsa.PublicKey) transport {
	return &rlpxTransport{conn: rlpx.NewConn(conn, dialDest)}
}

func (t *rlpxTransport) ReadMsg() (Msg, error) {
	t.rmu.Lock()
	defer t.rmu.Unlock()

	var msg Msg
	t.conn.SetReadDeadline(time.Now().Add(frameReadTimeout))
	code, data, wireSize, err := t.conn.Read()
	if err == nil {
		msg = Msg{
			ReceivedAt: time.Now(),
			Code:       code,
			Size:       uint32(len(data)),
			meterSize:  uint32(wireSize),
			Payload:    bytes.NewReader(data),
		}
	}
	return msg, err
}

func (t *rlpxTransport) WriteMsg(msg Msg) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()

	
	t.wbuf.Reset()
	if _, err := io.CopyN(&t.wbuf, msg.Payload, int64(msg.Size)); err != nil {
		return err
	}

	
	t.conn.SetWriteDeadline(time.Now().Add(frameWriteTimeout))
	size, err := t.conn.Write(msg.Code, t.wbuf.Bytes())
	if err != nil {
		return err
	}

	
	msg.meterSize = size
	if metrics.Enabled && msg.meterCap.Name != "" { 
		m := fmt.Sprintf("%s/%s/%d/%#02x", egressMeterName, msg.meterCap.Name, msg.meterCap.Version, msg.meterCode)
		metrics.GetOrRegisterMeter(m, nil).Mark(int64(msg.meterSize))
		metrics.GetOrRegisterMeter(m+"/packets", nil).Mark(1)
	}
	return nil
}

func (t *rlpxTransport) close(err error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()

	
	
	
	if t.conn != nil {
		if r, ok := err.(DiscReason); ok && r != DiscNetworkError {
			deadline := time.Now().Add(discWriteTimeout)
			if err := t.conn.SetWriteDeadline(deadline); err == nil {
				
				t.wbuf.Reset()
				rlp.Encode(&t.wbuf, []DiscReason{r})
				t.conn.Write(discMsg, t.wbuf.Bytes())
			}
		}
	}
	t.conn.Close()
}

func (t *rlpxTransport) doEncHandshake(prv *ecdsa.PrivateKey) (*ecdsa.PublicKey, error) {
	t.conn.SetDeadline(time.Now().Add(handshakeTimeout))
	return t.conn.Handshake(prv)
}

func (t *rlpxTransport) doProtoHandshake(our *protoHandshake) (their *protoHandshake, err error) {
	
	
	
	
	werr := make(chan error, 1)
	go func() { werr <- Send(t, handshakeMsg, our) }()
	if their, err = readProtocolHandshake(t); err != nil {
		<-werr 
		return nil, err
	}
	if err := <-werr; err != nil {
		return nil, fmt.Errorf("write error: %v", err)
	}
	
	t.conn.SetSnappy(their.Version >= snappyProtocolVersion)

	return their, nil
}

func readProtocolHandshake(rw MsgReader) (*protoHandshake, error) {
	msg, err := rw.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Size > baseProtocolMaxMsgSize {
		return nil, fmt.Errorf("message too big")
	}
	if msg.Code == discMsg {
		
		
		
		
		var reason [1]DiscReason
		rlp.Decode(msg.Payload, &reason)
		return nil, reason[0]
	}
	if msg.Code != handshakeMsg {
		return nil, fmt.Errorf("expected handshake, got %x", msg.Code)
	}
	var hs protoHandshake
	if err := msg.Decode(&hs); err != nil {
		return nil, err
	}
	if len(hs.ID) != 64 || !bitutil.TestBytes(hs.ID) {
		return nil, DiscInvalidIdentity
	}
	return &hs, nil
}

















package scwallet

import (
	"bytes"
	"encoding/binary"
	"fmt"
)


type commandAPDU struct {
	Cla, Ins, P1, P2 uint8  
	Data             []byte 
	Le               uint8  
}


func (ca commandAPDU) serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, ca.Cla); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, ca.Ins); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, ca.P1); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, ca.P2); err != nil {
		return nil, err
	}
	if len(ca.Data) > 0 {
		if err := binary.Write(buf, binary.BigEndian, uint8(len(ca.Data))); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.BigEndian, ca.Data); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buf, binary.BigEndian, ca.Le); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}


type responseAPDU struct {
	Data     []byte 
	Sw1, Sw2 uint8  
}


func (ra *responseAPDU) deserialize(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("can not deserialize data: payload too short (%d < 2)", len(data))
	}

	ra.Data = make([]byte, len(data)-2)

	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.BigEndian, &ra.Data); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &ra.Sw1); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &ra.Sw2); err != nil {
		return err
	}
	return nil
}

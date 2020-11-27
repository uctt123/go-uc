















package enr

import (
	"fmt"
	"io"
	"net"

	"github.com/ethereum/go-ethereum/rlp"
)






type Entry interface {
	ENRKey() string
}

type generic struct {
	key   string
	value interface{}
}

func (g generic) ENRKey() string { return g.key }

func (g generic) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, g.value)
}

func (g *generic) DecodeRLP(s *rlp.Stream) error {
	return s.Decode(g.value)
}




func WithEntry(k string, v interface{}) Entry {
	return &generic{key: k, value: v}
}


type TCP uint16

func (v TCP) ENRKey() string { return "tcp" }


type TCP6 uint16

func (v TCP6) ENRKey() string { return "tcp6" }


type UDP uint16

func (v UDP) ENRKey() string { return "udp" }


type UDP6 uint16

func (v UDP6) ENRKey() string { return "udp6" }


type ID string

const IDv4 = ID("v4") 

func (v ID) ENRKey() string { return "id" }




type IP net.IP

func (v IP) ENRKey() string {
	if net.IP(v).To4() == nil {
		return "ip6"
	}
	return "ip"
}


func (v IP) EncodeRLP(w io.Writer) error {
	if ip4 := net.IP(v).To4(); ip4 != nil {
		return rlp.Encode(w, ip4)
	}
	if ip6 := net.IP(v).To16(); ip6 != nil {
		return rlp.Encode(w, ip6)
	}
	return fmt.Errorf("invalid IP address: %v", net.IP(v))
}


func (v *IP) DecodeRLP(s *rlp.Stream) error {
	if err := s.Decode((*net.IP)(v)); err != nil {
		return err
	}
	if len(*v) != 4 && len(*v) != 16 {
		return fmt.Errorf("invalid IP address, want 4 or 16 bytes: %v", *v)
	}
	return nil
}


type IPv4 net.IP

func (v IPv4) ENRKey() string { return "ip" }


func (v IPv4) EncodeRLP(w io.Writer) error {
	ip4 := net.IP(v).To4()
	if ip4 == nil {
		return fmt.Errorf("invalid IPv4 address: %v", net.IP(v))
	}
	return rlp.Encode(w, ip4)
}


func (v *IPv4) DecodeRLP(s *rlp.Stream) error {
	if err := s.Decode((*net.IP)(v)); err != nil {
		return err
	}
	if len(*v) != 4 {
		return fmt.Errorf("invalid IPv4 address, want 4 bytes: %v", *v)
	}
	return nil
}


type IPv6 net.IP

func (v IPv6) ENRKey() string { return "ip6" }


func (v IPv6) EncodeRLP(w io.Writer) error {
	ip6 := net.IP(v).To16()
	if ip6 == nil {
		return fmt.Errorf("invalid IPv6 address: %v", net.IP(v))
	}
	return rlp.Encode(w, ip6)
}


func (v *IPv6) DecodeRLP(s *rlp.Stream) error {
	if err := s.Decode((*net.IP)(v)); err != nil {
		return err
	}
	if len(*v) != 16 {
		return fmt.Errorf("invalid IPv6 address, want 16 bytes: %v", *v)
	}
	return nil
}


type KeyError struct {
	Key string
	Err error
}


func (err *KeyError) Error() string {
	if err.Err == errNotFound {
		return fmt.Sprintf("missing ENR key %q", err.Key)
	}
	return fmt.Sprintf("ENR key %q: %v", err.Key, err.Err)
}



func IsNotFound(err error) bool {
	kerr, ok := err.(*KeyError)
	return ok && kerr.Err == errNotFound
}

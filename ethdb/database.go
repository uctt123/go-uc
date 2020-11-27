
















package ethdb

import "io"


type KeyValueReader interface {
	
	Has(key []byte) (bool, error)

	
	Get(key []byte) ([]byte, error)
}


type KeyValueWriter interface {
	
	Put(key []byte, value []byte) error

	
	Delete(key []byte) error
}


type Stater interface {
	
	Stat(property string) (string, error)
}


type Compacter interface {
	
	
	
	
	
	
	
	Compact(start []byte, limit []byte) error
}



type KeyValueStore interface {
	KeyValueReader
	KeyValueWriter
	Batcher
	Iteratee
	Stater
	Compacter
	io.Closer
}


type AncientReader interface {
	
	
	HasAncient(kind string, number uint64) (bool, error)

	
	Ancient(kind string, number uint64) ([]byte, error)

	
	Ancients() (uint64, error)

	
	AncientSize(kind string) (uint64, error)
}


type AncientWriter interface {
	
	
	AppendAncient(number uint64, hash, header, body, receipt, td []byte) error

	
	TruncateAncients(n uint64) error

	
	Sync() error
}



type Reader interface {
	KeyValueReader
	AncientReader
}



type Writer interface {
	KeyValueWriter
	AncientWriter
}



type AncientStore interface {
	AncientReader
	AncientWriter
	io.Closer
}



type Database interface {
	Reader
	Writer
	Batcher
	Iteratee
	Stater
	Compacter
	io.Closer
}

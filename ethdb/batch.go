















package ethdb



const IdealBatchSize = 100 * 1024



type Batch interface {
	KeyValueWriter

	
	ValueSize() int

	
	Write() error

	
	Reset()

	
	Replay(w KeyValueWriter) error
}


type Batcher interface {
	
	
	NewBatch() Batch
}

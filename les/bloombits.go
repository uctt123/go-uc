















package les

import (
	"time"

	"github.com/ethereum/go-ethereum/common/bitutil"
	"github.com/ethereum/go-ethereum/light"
)

const (
	
	
	bloomServiceThreads = 16

	
	
	bloomFilterThreads = 3

	
	
	bloomRetrievalBatch = 16

	
	
	bloomRetrievalWait = time.Microsecond * 100
)



func (eth *LightEthereum) startBloomHandlers(sectionSize uint64) {
	for i := 0; i < bloomServiceThreads; i++ {
		go func() {
			defer eth.wg.Done()
			for {
				select {
				case <-eth.closeCh:
					return

				case request := <-eth.bloomRequests:
					task := <-request
					task.Bitsets = make([][]byte, len(task.Sections))
					compVectors, err := light.GetBloomBits(task.Context, eth.odr, task.Bit, task.Sections)
					if err == nil {
						for i := range task.Sections {
							if blob, err := bitutil.DecompressBytes(compVectors[i], int(sectionSize/8)); err == nil {
								task.Bitsets[i] = blob
							} else {
								task.Error = err
							}
						}
					} else {
						task.Error = err
					}
					request <- task
				}
			}
		}()
	}
}

















package snapshot

import (
	"bytes"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)





func wipeSnapshot(db ethdb.KeyValueStore, full bool) chan struct{} {
	
	if full {
		rawdb.DeleteSnapshotRoot(db)
	}
	
	wiper := make(chan struct{}, 1)
	go func() {
		if err := wipeContent(db); err != nil {
			log.Error("Failed to wipe state snapshot", "err", err) 
			return
		}
		close(wiper)
	}()
	return wiper
}






func wipeContent(db ethdb.KeyValueStore) error {
	if err := wipeKeyRange(db, "accounts", rawdb.SnapshotAccountPrefix, len(rawdb.SnapshotAccountPrefix)+common.HashLength); err != nil {
		return err
	}
	if err := wipeKeyRange(db, "storage", rawdb.SnapshotStoragePrefix, len(rawdb.SnapshotStoragePrefix)+2*common.HashLength); err != nil {
		return err
	}
	
	start := time.Now()

	log.Info("Compacting snapshot account area ")
	end := common.CopyBytes(rawdb.SnapshotAccountPrefix)
	end[len(end)-1]++

	if err := db.Compact(rawdb.SnapshotAccountPrefix, end); err != nil {
		return err
	}
	log.Info("Compacting snapshot storage area ")
	end = common.CopyBytes(rawdb.SnapshotStoragePrefix)
	end[len(end)-1]++

	if err := db.Compact(rawdb.SnapshotStoragePrefix, end); err != nil {
		return err
	}
	log.Info("Compacted snapshot area in database", "elapsed", common.PrettyDuration(time.Since(start)))

	return nil
}



func wipeKeyRange(db ethdb.KeyValueStore, kind string, prefix []byte, keylen int) error {
	
	var (
		batch = db.NewBatch()
		items int
	)
	
	start, logged := time.Now(), time.Now()

	it := db.NewIterator(prefix, nil)
	for it.Next() {
		
		key := it.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		if len(key) != keylen {
			continue
		}
		
		batch.Delete(key)
		items++

		if items%10000 == 0 {
			
			it.Release()
			if err := batch.Write(); err != nil {
				return err
			}
			batch.Reset()
			seekPos := key[len(prefix):]
			it = db.NewIterator(prefix, seekPos)

			if time.Since(logged) > 8*time.Second {
				log.Info("Deleting state snapshot leftovers", "kind", kind, "wiped", items, "elapsed", common.PrettyDuration(time.Since(start)))
				logged = time.Now()
			}
		}
	}
	it.Release()
	if err := batch.Write(); err != nil {
		return err
	}
	log.Info("Deleted state snapshot leftovers", "kind", kind, "wiped", items, "elapsed", common.PrettyDuration(time.Since(start)))
	return nil
}

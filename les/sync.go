















package les

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
)

var errInvalidCheckpoint = errors.New("invalid advertised checkpoint")

const (
	
	
	lightSync = iota

	
	legacyCheckpointSync

	
	
	checkpointSync
)









func (h *clientHandler) validateCheckpoint(peer *serverPeer) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	
	cp := peer.checkpoint
	header, err := light.GetUntrustedHeaderByNumber(ctx, h.backend.odr, peer.checkpointNumber, peer.id)
	if err != nil {
		return err
	}
	
	logs, err := light.GetUntrustedBlockLogs(ctx, h.backend.odr, header)
	if err != nil {
		return err
	}
	events := h.backend.oracle.Contract().LookupCheckpointEvents(logs, cp.SectionIndex, cp.Hash())
	if len(events) == 0 {
		return errInvalidCheckpoint
	}
	var (
		index      = events[0].Index
		hash       = events[0].CheckpointHash
		signatures [][]byte
	)
	for _, event := range events {
		signatures = append(signatures, append(event.R[:], append(event.S[:], event.V)...))
	}
	valid, signers := h.backend.oracle.VerifySigners(index, hash, signatures)
	if !valid {
		return errInvalidCheckpoint
	}
	log.Warn("Verified advertised checkpoint", "peer", peer.id, "signers", len(signers))
	return nil
}


func (h *clientHandler) synchronise(peer *serverPeer) {
	
	if peer == nil {
		return
	}
	
	latest := h.backend.blockchain.CurrentHeader()
	currentTd := rawdb.ReadTd(h.backend.chainDb, latest.Hash(), latest.Number.Uint64())
	if currentTd != nil && peer.Td().Cmp(currentTd) < 0 {
		return
	}
	
	
	
	
	
	
	
	
	
	
	
	var checkpoint = &peer.checkpoint
	var hardcoded bool
	if h.checkpoint != nil && h.checkpoint.SectionIndex >= peer.checkpoint.SectionIndex {
		checkpoint = h.checkpoint 
		hardcoded = true
	}
	
	
	
	
	
	
	
	
	mode := checkpointSync
	switch {
	case checkpoint.Empty():
		mode = lightSync
		log.Debug("Disable checkpoint syncing", "reason", "empty checkpoint")
	case latest.Number.Uint64() >= (checkpoint.SectionIndex+1)*h.backend.iConfig.ChtSize-1:
		mode = lightSync
		log.Debug("Disable checkpoint syncing", "reason", "local chain beyond the checkpoint")
	case hardcoded:
		mode = legacyCheckpointSync
		log.Debug("Disable checkpoint syncing", "reason", "checkpoint is hardcoded")
	case h.backend.oracle == nil || !h.backend.oracle.IsRunning():
		if h.checkpoint == nil {
			mode = lightSync 
		} else {
			checkpoint = h.checkpoint
			mode = legacyCheckpointSync
		}
		log.Debug("Disable checkpoint syncing", "reason", "checkpoint syncing is not activated")
	}
	
	defer func() {
		if h.syncDone != nil {
			h.syncDone()
		}
	}()
	start := time.Now()
	if mode == checkpointSync || mode == legacyCheckpointSync {
		
		if mode == checkpointSync {
			if err := h.validateCheckpoint(peer); err != nil {
				log.Debug("Failed to validate checkpoint", "reason", err)
				h.removePeer(peer.id)
				return
			}
			h.backend.blockchain.AddTrustedCheckpoint(checkpoint)
		}
		log.Debug("Checkpoint syncing start", "peer", peer.id, "checkpoint", checkpoint.SectionIndex)

		
		
		
		
		
		
		
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		if !checkpoint.Empty() && !h.backend.blockchain.SyncCheckpoint(ctx, checkpoint) {
			log.Debug("Sync checkpoint failed")
			h.removePeer(peer.id)
			return
		}
	}
	
	if err := h.downloader.Synchronise(peer.id, peer.Head(), peer.Td(), downloader.LightSync); err != nil {
		log.Debug("Synchronise failed", "reason", err)
		return
	}
	log.Debug("Synchronise finished", "elapsed", common.PrettyDuration(time.Since(start)))
}

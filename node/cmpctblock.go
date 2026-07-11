// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/database"
	"github.com/modelos/modelos/node/peer"
	"github.com/modelos/modelos/node/wire"
)

// Compact-block relay (BIP-152, modelOS AuxPoW extension). This is PURELY
// ADDITIVE and feature-negotiated: it only runs between peers that negotiated
// sendcmpct, and on ANY failure it falls back to the existing full-block getdata
// path (OnBlock), which is left completely unchanged. Old peers never negotiate
// compact blocks and are unaffected; they ignore these messages (see peer.go).

// maxPendingCmpctBlocks bounds per-peer in-flight compact-block reconstructions
// so a peer cannot exhaust memory by sending many cmpctblocks without blocktxn.
const maxPendingCmpctBlocks = 16

// pendingCmpctTTL is how long a partial reconstruction is kept while awaiting its
// blocktxn before it is evicted (a peer that never answers must not pin memory).
const pendingCmpctTTL = 30 * time.Second

// minCmpctInterval rate-limits the expensive per-cmpctblock mempool sweep per
// peer. Legitimate blocks arrive ~once per target interval (194s), so a 1s floor
// is generous while defeating a flood of cmpctblocks aimed at CPU amplification.
const minCmpctInterval = time.Second

// pendingCmpctBlock is a compact block partially reconstructed from the mempool,
// waiting on a blocktxn to fill the transactions we were missing.
type pendingCmpctBlock struct {
	header  wire.MsgHeader
	auxPow  *wire.AuxPowData
	txns    []*wire.MsgTx // full tx list; missing slots are nil until blocktxn
	missing []uint32      // absolute indexes still needed, in request order
	created time.Time     // for TTL eviction
}

// OnSendCmpct handles a sendcmpct message. The peer's preference (whether it
// wants compact blocks and which version) is recorded on the peer itself; this
// handler only logs. Unknown/older versions are already filtered in peer.go.
func (sp *serverPeer) OnSendCmpct(_ *peer.Peer, msg *wire.MsgSendCmpct) {
	srvrLog.Debugf("Peer %s sendcmpct: announce=%v version=%d", sp, msg.Announce, msg.Version)
}

// OnGetBlockTxn handles a getblocktxn request: return the requested transactions
// (by absolute index) from a block we have, as a blocktxn. If we don't have the
// block, an index is out of range, an index is duplicated/non-increasing, or the
// response would exceed the max payload, the request is dropped (the requester
// falls back to getdata). The duplicate/size caps prevent a memory-exhaustion
// attack where a peer requests the same large tx tens of thousands of times.
func (sp *serverPeer) OnGetBlockTxn(_ *peer.Peer, msg *wire.MsgGetBlockTxn) {
	msgBlock, err := sp.server.fetchMsgBlock(&msg.BlockHash)
	if err != nil {
		srvrLog.Debugf("getblocktxn for unknown block %s from %s", msg.BlockHash, sp)
		return
	}
	numTx := len(msgBlock.Transactions)
	// A request can never legitimately ask for more txns than the block has.
	if len(msg.Indexes) > numTx {
		srvrLog.Debugf("getblocktxn requests %d indexes but block %s has %d txns from %s",
			len(msg.Indexes), msg.BlockHash, numTx, sp)
		return
	}
	txns := make([]*wire.MsgTx, 0, len(msg.Indexes))
	var lastIdx int64 = -1
	var totalBytes int
	for _, idx := range msg.Indexes {
		// Require strictly increasing indexes: rejects out-of-range AND duplicate
		// indexes in one check, so the response can never repeat a transaction.
		if int(idx) >= numTx || int64(idx) <= lastIdx {
			srvrLog.Debugf("getblocktxn bad index %d (last %d, ntx %d) block %s from %s",
				idx, lastIdx, numTx, msg.BlockHash, sp)
			return
		}
		lastIdx = int64(idx)
		tx := msgBlock.Transactions[idx]
		totalBytes += tx.SerializeSize()
		if totalBytes > int(wire.MaxMessagePayload) {
			srvrLog.Debugf("getblocktxn response would exceed max payload, block %s from %s",
				msg.BlockHash, sp)
			return
		}
		txns = append(txns, tx)
	}
	// WITNESS encoding is mandatory: the requested txns include the coinbase, whose witness (the 32-byte
	// witness reserved value) the requester needs to rebuild a block that passes the witness-commitment
	// check. QueueMessage defaults to BaseEncoding, which would strip the witness → the reconstructed block
	// fails with "coinbase has 0 items in its witness stack" and the whole compact-block sync stalls.
	sp.QueueMessageWithEncoding(wire.NewMsgBlockTxn(msg.BlockHash, txns), nil, wire.WitnessEncoding)
}

// OnCmpctBlock handles an incoming compact block: reconstruct the full block from
// our mempool + the prefilled txns. If complete, hand it to the sync manager as a
// normal block. If transactions are missing, request them via getblocktxn and
// hold the partial reconstruction. On any error, do nothing — normal block
// announcement/getdata still delivers the block (additive, never blocking).
func (sp *serverPeer) OnCmpctBlock(_ *peer.Peer, msg *wire.MsgCmpctBlock) {
	blockHash := msg.BlockHash()
	sp.AddKnownInventory(wire.NewInvVect(wire.InvTypeBlock, &blockHash))

	// If we already have this block, ignore.
	if have, err := sp.server.chain.HaveBlock(&blockHash); err == nil && have {
		return
	}

	// Cheap gate BEFORE the expensive mempool sweep: the block must connect to a
	// block we already know (its previous block is in our index). This drops
	// fabricated/disconnected headers that would otherwise force a full mempool
	// scan. Combined with the per-peer rate limit below, it bounds the CPU an
	// attacker can amplify from a tiny cmpctblock.
	if have, err := sp.server.chain.HaveBlock(&msg.Header.BlockHeader.PrevBlock); err != nil || !have {
		sp.requestFullBlock(&blockHash) // unknown parent: fetch normally instead
		return
	}

	// Rate-limit the reconstruction sweep per peer to bound the CPU an attacker
	// could amplify with a flood of cmpctblocks. We deliberately do NOT fall back
	// to getdata here — that would re-enable exactly the amplification the limit
	// exists to stop. Dropping is safe: legitimate blocks are ~194s apart (far
	// above the 1s floor), and any genuinely-new block is still delivered by the
	// getdata fallback on the NEXT (rate-permitted) cmpctblock or by another
	// peer's announcement. This only ever triggers under a burst/attack.
	sp.cmpctPendingMtx.Lock()
	if time.Since(sp.lastCmpctTime) < minCmpctInterval {
		sp.cmpctPendingMtx.Unlock()
		return
	}
	sp.lastCmpctTime = time.Now()
	sp.evictStalePendingLocked()
	sp.cmpctPendingMtx.Unlock()

	total := len(msg.ShortIDs) + len(msg.Prefilled)
	if total == 0 || total > wire.MaxTransactionsPerBlock {
		return
	}
	txns := make([]*wire.MsgTx, total)

	// Place prefilled txns at their absolute indexes.
	prefilledAt := make(map[int]bool, len(msg.Prefilled))
	for _, p := range msg.Prefilled {
		if int(p.Index) >= total {
			sp.requestFullBlock(&blockHash) // malformed; fetch the full block
			return
		}
		txns[p.Index] = p.Tx
		prefilledAt[int(p.Index)] = true
	}

	// Build a short-ID → tx index from the mempool under this block's key.
	key := wire.ShortIDKey(&msg.Header.BlockHeader, msg.Nonce)
	shortToTx := make(map[wire.ShortID]*wire.MsgTx)
	for _, desc := range sp.server.txMemPool.TxDescs() {
		wtxid := desc.Tx.MsgTx().WitnessHash()
		shortToTx[wire.ComputeShortID(&wtxid, &key)] = desc.Tx.MsgTx()
	}

	// Fill the non-prefilled slots in order from the short IDs.
	var missing []uint32
	sidPos := 0
	for i := 0; i < total; i++ {
		if prefilledAt[i] {
			continue
		}
		if sidPos >= len(msg.ShortIDs) {
			sp.requestFullBlock(&blockHash) // count mismatch; fetch the full block
			return
		}
		if tx, ok := shortToTx[msg.ShortIDs[sidPos]]; ok {
			txns[i] = tx
		} else {
			missing = append(missing, uint32(i))
		}
		sidPos++
	}

	if len(missing) == 0 {
		sp.assembleAndProcess(&msg.Header, msg.AuxPow, txns)
		return
	}

	// Missing transactions: hold the partial reconstruction and request them.
	sp.cmpctPendingMtx.Lock()
	if sp.cmpctPending == nil {
		sp.cmpctPending = make(map[chainhash.Hash]*pendingCmpctBlock)
	}
	if len(sp.cmpctPending) >= maxPendingCmpctBlocks {
		sp.cmpctPendingMtx.Unlock()
		sp.requestFullBlock(&blockHash) // too many in flight; fetch the full block
		return
	}
	sp.cmpctPending[blockHash] = &pendingCmpctBlock{
		header:  msg.Header,
		auxPow:  msg.AuxPow,
		txns:    txns,
		missing: missing,
		created: time.Now(),
	}
	sp.cmpctPendingMtx.Unlock()

	sp.QueueMessage(wire.NewMsgGetBlockTxn(blockHash, missing), nil)
}

// OnBlockTxn completes a pending compact-block reconstruction with the missing
// transactions and hands the full block to the sync manager.
func (sp *serverPeer) OnBlockTxn(_ *peer.Peer, msg *wire.MsgBlockTxn) {
	sp.cmpctPendingMtx.Lock()
	pending := sp.cmpctPending[msg.BlockHash]
	if pending != nil {
		delete(sp.cmpctPending, msg.BlockHash)
	}
	sp.evictStalePendingLocked() // also reclaim any others gone stale
	sp.cmpctPendingMtx.Unlock()
	if pending == nil {
		return // unsolicited or already handled
	}

	if len(msg.Transactions) != len(pending.missing) {
		sp.requestFullBlock(&msg.BlockHash) // mismatch; fetch the full block
		return
	}
	for i, idx := range pending.missing {
		if int(idx) >= len(pending.txns) {
			sp.requestFullBlock(&msg.BlockHash)
			return
		}
		pending.txns[idx] = msg.Transactions[i]
	}
	sp.assembleAndProcess(&pending.header, pending.auxPow, pending.txns)
}

// evictStalePendingLocked drops pending reconstructions older than pendingCmpctTTL
// so a peer that sends a cmpctblock but never answers the getblocktxn cannot pin
// memory. Caller must hold cmpctPendingMtx.
func (sp *serverPeer) evictStalePendingLocked() {
	now := time.Now()
	for h, p := range sp.cmpctPending {
		if now.Sub(p.created) > pendingCmpctTTL {
			delete(sp.cmpctPending, h)
		}
	}
}

// requestFullBlock falls back to fetching the full block via the normal getdata
// path when compact-block reconstruction can't complete (unknown parent,
// malformed, mismatch, or a short-ID collision). This goes through the sync
// manager so the block is properly request-accounted and not treated as
// unrequested. Best-effort: if we already have the block, it's a no-op.
func (sp *serverPeer) requestFullBlock(hash *chainhash.Hash) {
	if have, err := sp.server.chain.HaveBlock(hash); err == nil && have {
		return
	}
	sp.server.syncManager.RequestBlock(sp.Peer, hash)
}

// assembleAndProcess builds a full MsgBlock from the header, optional AuxPoW, and
// the complete transaction list, then hands it to the sync manager exactly as a
// full-block receive would. If any tx slot is still nil, it bails (the block
// arrives via the normal path). The reconstructed block is byte-identical to the
// original, so its hash, AuxPoW, and validation are unchanged.
func (sp *serverPeer) assembleAndProcess(header *wire.MsgHeader, auxPow *wire.AuxPowData, txns []*wire.MsgTx) {
	msgBlock := &wire.MsgBlock{MsgHeader: *header, AuxPow: auxPow, Transactions: make([]*wire.MsgTx, 0, len(txns))}
	for _, tx := range txns {
		if tx == nil {
			return // incomplete; let the normal relay path deliver it
		}
		msgBlock.Transactions = append(msgBlock.Transactions, tx)
	}

	// Serialize so btcutil.Block caches the same bytes a wire receive would.
	var buf bytes.Buffer
	if err := msgBlock.Serialize(&buf); err != nil {
		return
	}
	block := btcutil.NewBlockFromBlockAndBytes(msgBlock, buf.Bytes())
	// forceProcess: a compact-reconstructed block has no preceding getdata, so
	// the sync manager must not treat it as unrequested (which would drop it and
	// disconnect the honest peer). It still undergoes full consensus validation.
	sp.server.syncManager.QueueCompactBlock(block, sp.Peer, sp.blockProcessed)
	err := <-sp.blockProcessed
	// If the reconstruction validated to an INVALID block (most likely a short-ID
	// collision that placed a wrong-but-real mempool tx, breaking the merkle
	// root), fall back to fetching the authoritative full block via getdata —
	// matching Bitcoin Core's re-request on any failed compact reconstruction.
	// Without this, in a single-peer topology the block could be delayed until a
	// fresh announcement. requestFullBlock is a no-op if we now have the block.
	if err != nil {
		blockHash := block.Hash()
		sp.requestFullBlock(blockHash)
	}
}

// buildCompactBlock builds a cmpctblock for the block with the given hash to
// announce it to a high-bandwidth peer. Returns nil on any error (the caller
// then falls back to a normal inv/headers announcement). The coinbase (index 0)
// is always prefilled; every other transaction is sent as a 6-byte short ID.
func (s *server) buildCompactBlock(hash chainhash.Hash) *wire.MsgCmpctBlock {
	msgBlock, err := s.fetchMsgBlock(&hash)
	if err != nil || len(msgBlock.Transactions) == 0 {
		return nil
	}
	// A per-block random nonce keys the short IDs (grinding resistance).
	var nonceB [8]byte
	if _, err := rand.Read(nonceB[:]); err != nil {
		return nil
	}
	nonce := binary.LittleEndian.Uint64(nonceB[:])

	cb := wire.NewMsgCmpctBlock()
	cb.Header = msgBlock.MsgHeader
	cb.AuxPow = msgBlock.AuxPow
	cb.Nonce = nonce

	key := wire.ShortIDKey(&msgBlock.MsgHeader.BlockHeader, nonce)
	cb.Prefilled = []wire.PrefilledTx{{Index: 0, Tx: msgBlock.Transactions[0]}}
	cb.ShortIDs = make([]wire.ShortID, 0, len(msgBlock.Transactions)-1)
	for i := 1; i < len(msgBlock.Transactions); i++ {
		wtxid := msgBlock.Transactions[i].WitnessHash()
		cb.ShortIDs = append(cb.ShortIDs, wire.ComputeShortID(&wtxid, &key))
	}
	return cb
}

// fetchMsgBlock loads a full block by hash from the block database.
func (s *server) fetchMsgBlock(hash *chainhash.Hash) (*wire.MsgBlock, error) {
	var blockBytes []byte
	err := s.db.View(func(dbTx database.Tx) error {
		var err error
		blockBytes, err = dbTx.FetchBlock(hash)
		return err
	})
	if err != nil {
		return nil, err
	}
	var msgBlock wire.MsgBlock
	if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		return nil, err
	}
	return &msgBlock, nil
}

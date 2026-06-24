// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"bytes"
	"container/heap"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// txFeeRateEntry represents a single transaction in the fee-rate min-heap.
type txFeeRateEntry struct {
	txHash   chainhash.Hash
	feePerKB int64
	txSize   int64
	index    int // maintained by heap.Interface methods
}

// feeRateEntries implements heap.Interface as a min-heap ordered by feePerKB.
type feeRateEntries []*txFeeRateEntry

func (h feeRateEntries) Len() int { return len(h) }

func (h feeRateEntries) Less(i, j int) bool {
	if h[i].feePerKB == h[j].feePerKB {
		// Deterministic tie-breaking by raw hash bytes (no allocation).
		return bytes.Compare(h[i].txHash[:], h[j].txHash[:]) < 0
	}
	return h[i].feePerKB < h[j].feePerKB
}

func (h feeRateEntries) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *feeRateEntries) Push(x interface{}) {
	entry := x.(*txFeeRateEntry)
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *feeRateEntries) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil // avoid memory leak
	entry.index = -1
	*h = old[:n-1]
	return entry
}

// txFeeRateHeap wraps a min-heap of fee-rate entries with an index map for
// O(log n) removal by transaction hash.
type txFeeRateHeap struct {
	entries  feeRateEntries
	indexMap map[chainhash.Hash]*txFeeRateEntry
}

func newTxFeeRateHeap() *txFeeRateHeap {
	h := &txFeeRateHeap{
		entries:  make(feeRateEntries, 0),
		indexMap: make(map[chainhash.Hash]*txFeeRateEntry),
	}
	heap.Init(&h.entries)
	return h
}

// Len returns the number of entries in the heap.
func (h *txFeeRateHeap) Len() int {
	return h.entries.Len()
}

// Add inserts a new entry into the heap.
func (h *txFeeRateHeap) Add(txHash chainhash.Hash, feePerKB, txSize int64) {
	if _, exists := h.indexMap[txHash]; exists {
		return
	}
	entry := &txFeeRateEntry{
		txHash:   txHash,
		feePerKB: feePerKB,
		txSize:   txSize,
	}
	heap.Push(&h.entries, entry)
	h.indexMap[txHash] = entry
}

// PopMin removes and returns the entry with the lowest fee rate.
// Returns nil if the heap is empty.
func (h *txFeeRateHeap) PopMin() *txFeeRateEntry {
	if h.entries.Len() == 0 {
		return nil
	}
	entry := heap.Pop(&h.entries).(*txFeeRateEntry)
	delete(h.indexMap, entry.txHash)
	return entry
}

// Remove removes the entry for the given transaction hash.
// It is a no-op if the hash is not present.
func (h *txFeeRateHeap) Remove(txHash chainhash.Hash) {
	entry, exists := h.indexMap[txHash]
	if !exists {
		return
	}
	heap.Remove(&h.entries, entry.index)
	delete(h.indexMap, txHash)
}

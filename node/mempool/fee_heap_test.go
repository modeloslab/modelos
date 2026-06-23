// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"testing"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeHash creates a chainhash.Hash from a byte for testing.
func makeHash(b byte) chainhash.Hash {
	var h chainhash.Hash
	h[0] = b
	return h
}

func TestFeeRateHeap_PopMinOrdering(t *testing.T) {
	h := newTxFeeRateHeap()

	h.Add(makeHash(1), 5000, 200)
	h.Add(makeHash(2), 1000, 300)
	h.Add(makeHash(3), 3000, 100)
	h.Add(makeHash(4), 2000, 400)
	h.Add(makeHash(5), 4000, 150)

	require.Equal(t, 5, h.Len())

	wantOrder := []int64{1000, 2000, 3000, 4000, 5000}
	for i, want := range wantOrder {
		entry := h.PopMin()
		require.NotNil(t, entry, "PopMin returned nil at index %d", i)
		assert.Equal(t, want, entry.feePerKB, "wrong feePerKB at index %d", i)
	}

	assert.Equal(t, 0, h.Len())
	assert.Nil(t, h.PopMin(), "PopMin on empty heap should return nil")
}

func TestFeeRateHeap_RemoveByHash(t *testing.T) {
	h := newTxFeeRateHeap()

	h.Add(makeHash(1), 5000, 200)
	h.Add(makeHash(2), 1000, 300)
	h.Add(makeHash(3), 3000, 100)

	h.Remove(makeHash(3))

	require.Equal(t, 2, h.Len())

	got1 := h.PopMin()
	require.NotNil(t, got1)
	assert.Equal(t, int64(1000), got1.feePerKB)

	got2 := h.PopMin()
	require.NotNil(t, got2)
	assert.Equal(t, int64(5000), got2.feePerKB)
}

func TestFeeRateHeap_RemoveNonExistent(t *testing.T) {
	h := newTxFeeRateHeap()

	h.Add(makeHash(1), 1000, 200)
	h.Remove(makeHash(99))

	assert.Equal(t, 1, h.Len())
}

func TestFeeRateHeap_DuplicateAdd(t *testing.T) {
	h := newTxFeeRateHeap()

	h.Add(makeHash(1), 1000, 200)
	h.Add(makeHash(1), 2000, 300) // duplicate, should be ignored

	require.Equal(t, 1, h.Len())

	entry := h.PopMin()
	require.NotNil(t, entry)
	assert.Equal(t, int64(1000), entry.feePerKB, "duplicate add should not overwrite")
}

func TestFeeRateHeap_IndexConsistency(t *testing.T) {
	h := newTxFeeRateHeap()

	for i := byte(0); i < 20; i++ {
		h.Add(makeHash(i), int64(i)*100+100, int64(i)*10+50)
	}

	for i := byte(0); i < 20; i += 3 {
		h.Remove(makeHash(i))
	}

	poppedCount := h.Len() / 2
	for i := 0; i < poppedCount; i++ {
		h.PopMin()
	}

	for hash, entry := range h.indexMap {
		require.True(t, entry.index >= 0 && entry.index < h.entries.Len(),
			"entry for hash %v has out-of-range index %d", hash, entry.index)
		assert.Equal(t, entry, h.entries[entry.index],
			"indexMap/entries mismatch for hash %v", hash)
	}

	assert.Equal(t, h.entries.Len(), len(h.indexMap),
		"indexMap size should equal entries size")
}

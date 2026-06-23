// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"container/heap"
	"math/rand"
	"testing"
	"time"

	"github.com/modelos/modelos/node/blockchain"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
	"github.com/modelos/modelos/node/database"
	_ "github.com/modelos/modelos/node/database/ffldb"
	"github.com/modelos/modelos/node/txscript"
	"github.com/modelos/modelos/node/wire"
	"github.com/stretchr/testify/require"
)

// TestTxFeeHeap ensures the priority queue for transaction fees works as expected.
func TestTxFeeHeap(t *testing.T) {
	// Create some fake fee items that exercise the expected sort edge conditions.
	testItems := []*txPrioItem{
		{feePerKB: 5678},
		{feePerKB: 5678}, // Duplicate fee
		{feePerKB: 1234},
		{feePerKB: 1234}, // Duplicate fee
		{feePerKB: 10000},
		{feePerKB: 100},
		{feePerKB: 0},
	}

	// Add random data in addition to the edge conditions already manually specified.
	randSeed := rand.Int63()
	defer func() {
		if t.Failed() {
			t.Logf("Random numbers using seed: %v", randSeed)
		}
	}()
	prng := rand.New(rand.NewSource(randSeed))
	for i := 0; i < 1000; i++ {
		testItems = append(testItems, &txPrioItem{
			feePerKB: int64(prng.Float64() * btcutil.GrainPerMDL),
		})
	}

	// Test sorting by fee per KB (descending order - highest fee first).
	priorityQueue := newTxPriorityQueue(len(testItems))
	for _, item := range testItems {
		heap.Push(priorityQueue, item)
	}

	// Verify items are popped in descending fee order.
	prevFee := int64(1<<63 - 1) // Max int64
	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		require.LessOrEqual(t, prioItem.feePerKB, prevFee,
			"item %d: fee %d should be <= previous fee %d", i, prioItem.feePerKB, prevFee)
		prevFee = prioItem.feePerKB
	}
}

// emptyTxSource is a TxSource with no transactions, used to test template
// generation with an empty mempool.
type emptyTxSource struct{}

func (e *emptyTxSource) LastUpdated() time.Time               { return time.Time{} }
func (e *emptyTxSource) MiningDescs() []*TxDesc               { return nil }
func (e *emptyTxSource) HaveTransaction(*chainhash.Hash) bool { return false }

// TestNewBlockTemplateNilAddress exercises the full NewBlockTemplate code path
// with no mining address (payToAddress == nil). This is the exact path taken by
// the getblocktemplate RPC when useCoinbaseValue is true (the default), which
// builds a placeholder coinbase that is later replaced by external mining
// software. The coinbase must comply with Taproot-only consensus rules.
func TestNewBlockTemplateNilAddress(t *testing.T) {
	params := chaincfg.SimNetParams

	db, err := database.Create("ffldb", t.TempDir(), wire.SimNet)
	require.NoError(t, err)
	defer db.Close()

	timeSource := blockchain.NewMedianTime()
	sigCache := txscript.NewSigCache(100)
	hashCache := txscript.NewHashCache(100)

	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: &params,
		TimeSource:  timeSource,
		SigCache:    sigCache,
		HashCache:   hashCache,
	})
	require.NoError(t, err)

	generator := NewBlkTmplGenerator(
		&Policy{
			BlockMinVsize: 0,
			BlockMaxVsize: blockchain.MaxBlockVsize,
		},
		&params,
		&emptyTxSource{},
		chain,
		timeSource,
		sigCache,
		hashCache,
	)

	template, err := generator.NewBlockTemplate(nil)
	require.NoError(t, err, "NewBlockTemplate(nil) should succeed with a placeholder coinbase")
	require.NotNil(t, template.Block)
	require.NotEmpty(t, template.Block.Transactions)

	coinbaseTx := btcutil.NewTx(template.Block.Transactions[0])
	require.NoError(t, blockchain.CheckTransactionSanity(coinbaseTx, false),
		"placeholder coinbase must pass Taproot-only consensus validation")
}

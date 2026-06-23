package integration

import (
	"testing"

	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func TestInvalidateAndReconsiderBlock(t *testing.T) {
	r := require.New(t)

	// Set up simnet chain.
	rpc, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	r.NoError(err, "Unable to create primary harness")

	err = rpc.SetUp(true, 0)
	r.NoError(err, "Unable to setup test chain")
	defer rpc.TearDown()

	// Generate 4 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	_, err = rpc.Client.Generate(4)
	r.NoError(err)

	// Cache the active tip hash.
	block4ActiveTipHash, err := rpc.Client.GetBestBlockHash()
	r.NoError(err)

	// Cache block 1 hash as this will be our chaintip after we invalidate block 2.
	block1Hash, err := rpc.Client.GetBlockHash(1)
	r.NoError(err)

	// Invalidate block 2.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1                 (active)
	//                    \ -> 2 -> 3 -> 4  (invalid)
	block2Hash, err := rpc.Client.GetBlockHash(2)
	r.NoError(err)

	err = rpc.Client.InvalidateBlock(block2Hash)
	r.NoError(err)

	// Assert that block 1 is the active chaintip.
	bestHash, err := rpc.Client.GetBestBlockHash()
	r.NoError(err)
	r.Equal(*block1Hash, *bestHash, "Expected the best block hash to be block 1")

	// Generate 2 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a      (active)
	//                    \ -> 2  -> 3  -> 4 (invalid)
	_, err = rpc.Client.Generate(2)
	r.NoError(err)

	// Cache the active tip hash for the current active tip.
	block3aActiveTipHash, err := rpc.Client.GetBestBlockHash()
	r.NoError(err)

	tips, err := rpc.Client.GetChainTips()
	r.NoError(err)

	// Assert that there are two branches.
	r.Equal(2, len(tips), "Expected 2 chaintips")

	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			r.Equal("invalid", tip.Status, "Expected invalidated branch tip to be invalid")
		}
	}

	// Reconsider the invalidated block 2.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (valid-fork)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = rpc.Client.ReconsiderBlock(block2Hash)
	r.NoError(err)

	tips, err = rpc.Client.GetChainTips()
	r.NoError(err)

	// Assert that there are two branches.
	r.Equal(2, len(tips), "Expected 2 chaintips")

	var checkedTips int
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			r.Equal("active", tip.Status, "Expected the reconsidered branch tip to be active")
			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			r.Equal("valid-fork", tip.Status, "Expected invalidated branch tip to be valid-fork")
			checkedTips++
		}
	}

	r.Equal(2, checkedTips, "Expected to check 2 chaintips")

	// Invalidate block 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (invalid)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = rpc.Client.InvalidateBlock(block3aActiveTipHash)
	r.NoError(err)

	tips, err = rpc.Client.GetChainTips()
	r.NoError(err)

	// Assert that there are two branches.
	r.Equal(2, len(tips), "Expected 2 chaintips")

	checkedTips = 0
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			r.Equal("active", tip.Status, "Expected an active branch tip")
			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			r.Equal("invalid", tip.Status, "Expected the invalidated tip to be invalid")
			checkedTips++
		}
	}

	r.Equal(2, checkedTips, "Expected to check 2 chaintips")

	// Reconsider block 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (valid-fork)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = rpc.Client.ReconsiderBlock(block3aActiveTipHash)
	r.NoError(err)

	tips, err = rpc.Client.GetChainTips()
	r.NoError(err)

	// Assert that there are two branches.
	r.Equal(2, len(tips), "Expected 2 chaintips, got %d (tips: %v)", len(tips), tips)

	checkedTips = 0
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			r.Equal("active", tip.Status, "Expected an active branch tip")
			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			r.Equal("valid-fork", tip.Status, "Expected the reconsidered tip to be a valid-fork")
			checkedTips++
		}
	}

	r.Equal(2, checkedTips, "Expected to check 2 chaintips")
}

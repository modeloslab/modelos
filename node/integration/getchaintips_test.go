package integration

import (
	"testing"
	"time"

	"github.com/modelos/modelos/node/btcjson"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/integration/rpctest"
	"github.com/stretchr/testify/require"
)

// compareMultipleChainTips checks that all the expected chain tips are included in got chain tips and
// verifies that the got chain tip matches the expected chain tip.
func compareMultipleChainTips(r *require.Assertions, gotChainTips, expectedChainTips []*btcjson.GetChainTipsResult) {
	r.Equal(len(gotChainTips), len(expectedChainTips), "Expected %d chaintips but got %d", len(expectedChainTips), len(gotChainTips))

	gotChainTipsMap := make(map[string]btcjson.GetChainTipsResult)
	for _, gotChainTip := range gotChainTips {
		gotChainTipsMap[gotChainTip.Hash] = *gotChainTip
	}

	for _, expectedChainTip := range expectedChainTips {
		gotChainTip, found := gotChainTipsMap[expectedChainTip.Hash]
		r.True(found, "Couldn't find expected chaintip with hash %s", expectedChainTip.Hash)
		r.Equal(gotChainTip, *expectedChainTip)
	}
}

func TestGetChainTips(t *testing.T) {
	r := require.New(t)

	createBlock := func(block *btcutil.Block, txs []*btcutil.Tx, height int32, timestamp time.Time, addr btcutil.Address, chainParams *chaincfg.Params) *btcutil.Block {
		blk, err := rpctest.CreateBlock(block, txs, height, timestamp, addr, nil, chainParams)
		r.NoError(err)
		return blk
	}

	netParams := &chaincfg.SimNetParams

	// Create a test address to receive funds (Taproot address)
	addr, err := btcutil.NewAddressTaproot(
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
			0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
			0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20},
		netParams)
	r.NoError(err)

	// Derive block timestamps from the simnet genesis so the test isn't
	// tied to any specific wall-clock date.
	genesisTime := netParams.GenesisBlock.BlockHeader().Timestamp
	ts := func(offsetSecs int64) time.Time {
		return genesisTime.Add(time.Duration(offsetSecs) * time.Second)
	}

	// block1 is a block that builds on top of the simnet genesis block.
	block1 := createBlock(nil, nil, 1, ts(1), addr, netParams)

	// block2 is a block that builds on top of block1.
	block2 := createBlock(block1, nil, 1, ts(11), addr, netParams)

	// block3 is a block that builds on top of block2.
	block3 := createBlock(block2, nil, 1, ts(21), addr, netParams)

	// block4 is a block that builds on top of block3.
	block4 := createBlock(block3, nil, 1, ts(31), addr, netParams)

	// block2a is a block that builds on top of block1.
	block2a := createBlock(block1, nil, 1, ts(12), addr, netParams)

	// block3a is a block that builds on top of block2a.
	block3a := createBlock(block2a, nil, 1, ts(22), addr, netParams)

	// block4a is a block that builds on top of block3a.
	block4a := createBlock(block3a, nil, 1, ts(32), addr, netParams)

	// block5a is a block that builds on top of block4a.
	block5a := createBlock(block4a, nil, 1, ts(42), addr, netParams)

	// block4b is a block that builds on top of block3a.
	block4b := createBlock(block3a, nil, 1, ts(33), addr, netParams)

	// Set up simnet chain.
	rpc, err := rpctest.New(netParams, nil, nil, "")
	r.NoError(err, "Unable to create primary harness")
	r.NoError(rpc.SetUp(true, 0), "Unable to setup test chain: %v", err)
	defer rpc.TearDown()

	// Immediately call getchaintips after setting up simnet.
	gotChainTips, err := rpc.Client.GetChainTips()
	r.NoError(err)
	// We expect a single genesis block.
	expectedChainTips := []*btcjson.GetChainTipsResult{
		{
			Height:    0,
			Hash:      chaincfg.RegressionNetParams.GenesisHash.String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)

	// Submit 4 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	blocks := []*btcutil.Block{block1, block2, block3, block4}
	for _, block := range blocks {
		r.NoError(rpc.Client.SubmitBlock(block, nil))
	}

	gotChainTips, err = rpc.Client.GetChainTips()
	r.NoError(err)
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      block4.Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)

	// Submit 2 blocks that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4  (active)
	//                    \ -> 2a -> 3a       (valid-fork)
	blocks = []*btcutil.Block{block2a, block3a}
	for _, block := range blocks {
		r.NoError(rpc.Client.SubmitBlock(block, nil))
	}

	gotChainTips, err = rpc.Client.GetChainTips()
	r.NoError(err)
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      block4.Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    3,
			Hash:      block3a.Hash().String(),
			BranchLen: 2,
			Status:    "valid-fork",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)

	// Submit a single block that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4   (active)
	//                    \ -> 2a -> 3a -> 4a  (valid-fork)
	r.NoError(rpc.Client.SubmitBlock(block4a, nil))

	gotChainTips, err = rpc.Client.GetChainTips()
	r.NoError(err)
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      block4.Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      block4a.Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)

	// Submit a single block that changes the active branch to 5a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	r.NoError(rpc.Client.SubmitBlock(block5a, nil))
	gotChainTips, err = rpc.Client.GetChainTips()
	r.NoError(err)
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      block4.Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      block5a.Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)

	// Submit a single block that builds on top of 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	//                                \ -> 4b        (valid-fork)
	r.NoError(rpc.Client.SubmitBlock(block4b, nil))
	gotChainTips, err = rpc.Client.GetChainTips()
	r.NoError(err)
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      block4.Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      block5a.Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      block4b.Hash().String(),
			BranchLen: 1,
			Status:    "valid-fork",
		},
	}
	compareMultipleChainTips(r, gotChainTips, expectedChainTips)
}

// Copyright (c) 2026 The modelOS Authors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/wire"
)

// zkBlockFixtures mirrors testdata/zk_block_fixture.json, produced by
// gen_zk_block_fixture.py using the PRODUCTION pool's modelos_pool.coinbase
// (build_pool_coinbase) + the gateway block wire framing. Each fixture is a complete
// pool-assembled, coinbase-only height-1 ZK block.
type zkBlockFixtures struct {
	Fixtures []zkBlockFixture `json:"fixtures"`
}

type zkBlockFixture struct {
	Label      string `json:"label"`
	BlockHex   string `json:"block_hex"`
	Height     int32  `json:"height"`
	BitsHex    string `json:"bits_hex"`
	Segwit     bool   `json:"segwit"`
	CoinbHex   string `json:"coinbase_hex"`
	MerkleLEHx string `json:"merkle_root_le_hex"`
}

func loadZKBlockFixtures(t *testing.T) []zkBlockFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/zk_block_fixture.json")
	if err != nil {
		t.Fatalf("read fixture (run gen_zk_block_fixture.py): %v", err)
	}
	var fx zkBlockFixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(fx.Fixtures) == 0 {
		t.Fatal("fixture file has no entries")
	}
	return fx.Fixtures
}

func decodeFixtureBlock(t *testing.T, fx zkBlockFixture) *btcutil.Block {
	t.Helper()
	blockBytes, err := hex.DecodeString(fx.BlockHex)
	if err != nil {
		t.Fatalf("[%s] decode block hex: %v", fx.Label, err)
	}
	// Deserialize via the node's OWN wire decoder — this alone proves the pool's
	// cert|header(108)|varint|txns framing round-trips into the node's MsgBlock.
	var msg wire.MsgBlock
	if err := msg.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		t.Fatalf("[%s] node wire decode of pool block failed: %v", fx.Label, err)
	}
	return btcutil.NewBlock(&msg)
}

// TestZKBlock_PoolConstructed_PassesNodeValidation is the belt-and-suspenders
// cross-language check for the witness-share block-landing path: a block assembled by
// the PRODUCTION pool's coinbase + merkle + wire framing (Python) must pass the node's
// REAL validation (Go). It covers exactly the pool-specific surface the node enforces:
//
//   - wire decode of the cert|header|varint|txns framing
//   - merkle root: header.MerkleRoot == CalcMerkleRoot([coinbase]+txns) (validate.go:602)
//   - coinbase sanity + BIP34 height encoding (OP_1 for height 1)
//   - segwit witness commitment (ValidateWitnessCommitment), when present
//
// The plonky2 proof itself is intentionally skipped via BFNoPoWCheck — that path is the
// same generate_proof the gateway uses to land blocks today, and test_zk_block.py pins
// the jackpot at offset 152. If the pool's coinbase/merkle/framing ever drift from what
// the node accepts, this fails BEFORE any pod is spun up.
func TestZKBlock_PoolConstructed_PassesNodeValidation(t *testing.T) {
	params := &chaincfg.MainNetParams
	timeSource := NewMedianTime()

	for _, fx := range loadZKBlockFixtures(t) {
		fx := fx
		t.Run(fx.Label, func(t *testing.T) {
			block := decodeFixtureBlock(t, fx)

			// Full context-free sanity, minus the gateway-proven plonky2 verify.
			// The merkle-root match (the core of the coinbase fix) is checked here.
			if err := checkBlockSanity(block, params, timeSource, BFNoPoWCheck); err != nil {
				t.Fatalf("[%s] checkBlockSanity rejected pool-built block: %v", fx.Label, err)
			}

			// BIP34: the coinbase scriptSig must encode height 1 the way the node
			// re-derives it (ScriptBuilder.AddInt64 → OP_1). Contextual, so run it
			// explicitly here.
			coinbase := block.Transactions()[0]
			if err := CheckSerializedHeight(coinbase, fx.Height); err != nil {
				t.Fatalf("[%s] coinbase BIP34 height check failed: %v", fx.Label, err)
			}

			// Segwit commitment (only meaningful when present; for the legacy fixture
			// this returns nil because there is no witness data).
			if err := ValidateWitnessCommitment(block); err != nil {
				t.Fatalf("[%s] witness commitment validation failed: %v", fx.Label, err)
			}
		})
	}
}

// TestZKBlock_TamperedCoinbase_Rejected confirms the cross-check has teeth: mutating the
// coinbase (without updating the committed merkle root) must make the node's merkle
// check reject the block — i.e. the test would actually catch a coinbase/merkle drift.
func TestZKBlock_TamperedCoinbase_Rejected(t *testing.T) {
	params := &chaincfg.MainNetParams
	timeSource := NewMedianTime()

	for _, fx := range loadZKBlockFixtures(t) {
		fx := fx
		t.Run(fx.Label, func(t *testing.T) {
			blockBytes, err := hex.DecodeString(fx.BlockHex)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var msg wire.MsgBlock
			if err := msg.Deserialize(bytes.NewReader(blockBytes)); err != nil {
				t.Fatalf("wire decode: %v", err)
			}
			// Flip the coinbase's reward output value — changes its txid, so the
			// recomputed merkle root no longer matches the header's committed root.
			msg.Transactions[0].TxOut[0].Value ^= 1
			tampered := btcutil.NewBlock(&msg)

			err = checkBlockSanity(tampered, params, timeSource, BFNoPoWCheck)
			if err == nil {
				t.Fatalf("[%s] tampered coinbase accepted — merkle check has no teeth", fx.Label)
			}
			if rerr, ok := err.(RuleError); ok && rerr.ErrorCode != ErrBadMerkleRoot {
				t.Fatalf("[%s] expected ErrBadMerkleRoot, got %v", fx.Label, rerr.ErrorCode)
			}
		})
	}
}

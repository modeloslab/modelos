package blockchain

import (
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// ChainCtx is an interface that abstracts away blockchain parameters.
type ChainCtx interface {
	// ChainParams returns the chain's configured chaincfg.Params.
	ChainParams() *chaincfg.Params

	// VerifyCheckpoint returns whether the passed height and hash match
	// the checkpoint data. Not all instances of VerifyCheckpoint will use
	// this function for validation.
	VerifyCheckpoint(height int32, hash *chainhash.Hash) bool

	// FindPreviousCheckpoint returns the most recent checkpoint that we
	// have validated. Not all instances of FindPreviousCheckpoint will use
	// this function for validation.
	FindPreviousCheckpoint() (chaincfg.HeaderCtx, error)
}

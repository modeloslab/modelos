package blockchain

import (
	"github.com/modelos/modelos/node/btcutil"
)

// Virtual size ("vsize") is a consensus metric that measures the "weight" of
// transaction and block data, accounting for the discounted cost of Segregated
// Witness (SegWit) data. Unlike raw byte size, virtual size gives different
// weight to witness and non-witness data to incentivize moving signature data
// out of the main transaction body and into witness fields.
//
// The vsize formula is:
//
//     vsize = base_size + ceiling(witness_size / WitnessScaleFactor)
//
// where:
//   - base_size: number of bytes excluding all witness data (the "stripped" size)
//   - witness_size: total length in bytes of all witness data
//   - WitnessScaleFactor: 4 (by consensus rules)
//
// Practically, each byte of base data counts fully (1 vbyte/byte), and each
// byte of witness data counts as 0.25 vbytes. This incentivizes signature data
// to be carried in segregated witness fields, improving network throughput.

const (
	// MaxBlockVsize is the consensus-enforced maximum virtual size of a block (in vbytes),
	// not its actual physical byte size. This limit is defined by:
	//    baseSize + ceil(witnessSize/4) <= 1000000
	// This allows larger actual blocks when populated with more discounted witness data.
	MaxBlockVsize = 1000000

	// WitnessScaleFactor is the SegWit discount divisor. Each witness byte counts as
	// 1/WitnessScaleFactor (0.25) when determining vsize. This is enforced
	// throughout the system for block limits and fee calculations.
	WitnessScaleFactor = 4
)

// GetBlockVsize returns the block's virtual size in vbytes according to consensus rules.
// This is defined as the stripped (non-witness, non-certificate) size plus the witness
// size divided by WitnessScaleFactor, rounded up.
// NOTE: Certificate is NOT included in vsize - it's proof metadata, not block data.
func GetBlockVsize(blk *btcutil.Block) int64 {
	msgBlock := blk.MsgBlock()

	// SerializeSizeStripped excludes both witness and certificate (consensus base size)
	baseSize := msgBlock.SerializeSizeStripped()
	// Get just the witness data size
	witnessSize := msgBlock.SerializeWitnessSize()

	return CalcVsize(baseSize, witnessSize)
}

// GetTransactionVsize returns a transaction's virtual size in vbytes as per consensus
// rules.
func GetTransactionVsize(tx *btcutil.Tx) int64 {
	msgTx := tx.MsgTx()

	baseSize := msgTx.SerializeSizeStripped()
	witnessSize := msgTx.SerializeSize() - baseSize

	return CalcVsize(baseSize, witnessSize)
}

// CalcVsize calculates the vsize (virtual size) from the given base (non-witness) and witness sizes,
// in bytes, following consensus rules. Witness data is discounted using WitnessScaleFactor (4),
// such that each witness byte counts as only 0.25 vbytes. When dividing the witness size by
// WitnessScaleFactor, ceiling division is used—any partial witness "chunk" still counts as a
// full virtual byte. This ensures that even 1–3 witness bytes still consume a full vbyte of block space
// (i.e., to avoid undercounting small witness data).
func CalcVsize(baseSize int, witnessSize int) int64 {
	return int64(baseSize + (witnessSize+WitnessScaleFactor-1)/WitnessScaleFactor)
}

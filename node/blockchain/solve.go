// Copyright (c) 2025-2026 The Pearl Research Labs developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/modelos/modelos/node/wire"
	"github.com/modelos/modelos/node/zkpow"
)

// SolveBlock mines a ZKCertificate for the given block header.
// On SimNet, it returns a lightweight dummy certificate (no actual mining),
// matching the auto-skip of PoW verification in checkBlockSanity for SimNet.
// NOTE: This function modifies header.ProofCommitment to match the mined certificate.
func SolveBlock(header *wire.BlockHeader, net wire.PearlNet) (wire.BlockCertificate, error) {
	if net == wire.SimNet {
		return &wire.ZKCertificate{PublicData: [wire.PublicDataSize]byte{}, ProofData: []byte{0x00}}, nil
	}
	return zkpow.Mine(header)
}

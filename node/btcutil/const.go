// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

const (
	// GrainPerMDLCent is the number of grains in one MDL cent.
	GrainPerMDLCent = 1e6

	// GrainPerMDL is the number of grains in one MDL token.
	// 1 MDL = 100,000,000 grains (same denomination as Bitcoin satoshis).
	GrainPerMDL = 1e8

	// MaxGrain is the maximum transaction amount allowed in grains.
	// Set to 10× total supply as a consensus safety cap.
	MaxGrain = 21e7 * GrainPerMDL

	// MinInferenceFeeGrains is the minimum inference_tx fee (0.01 MDL).
	// Miners should prioritise inference_tx with higher fees for faster
	// inclusion and quicker result delivery to the requesting user.
	MinInferenceFeeGrains = int64(1_000_000) // 0.01 MDL
)

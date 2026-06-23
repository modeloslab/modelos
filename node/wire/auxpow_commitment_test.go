// Copyright (c) 2025-2026 The Pearl Research Labs developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"testing"

	"github.com/modelos/modelos/node/chaincfg/chainhash"
)

// TestContainsModelosCommitment verifies the merged-mining commitment scan,
// including the Namecoin/Dogecoin "multiple merged-mining headers" guard: the
// AuxPowMagic marker must appear exactly once or the commitment is rejected.
func TestContainsModelosCommitment(t *testing.T) {
	var sigma chainhash.Hash
	for i := range sigma {
		sigma[i] = byte(i + 1) // 0x01..0x20, deterministic non-zero hash
	}

	magic := AuxPowMagic[:]
	commit := append(append([]byte{}, magic...), sigma[:]...) // magic||sigma (36 bytes)

	tests := []struct {
		name      string
		scriptSig []byte
		want      int
	}{
		{
			name:      "magic+hash at offset 0",
			scriptSig: commit,
			want:      0,
		},
		{
			name:      "magic+hash at non-zero offset",
			scriptSig: append([]byte{0xaa, 0xbb, 0xcc}, commit...),
			want:      3,
		},
		{
			name:      "no magic present",
			scriptSig: []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			want:      -1,
		},
		{
			name:      "magic present but wrong hash",
			scriptSig: append(append([]byte{}, magic...), make([]byte, 32)...),
			want:      -1,
		},
		{
			name: "duplicate magic rejected (second magic after a valid commitment)",
			// valid commitment, then a second bare magic marker → must reject.
			scriptSig: append(append([]byte{}, commit...), magic...),
			want:      -1,
		},
		{
			name: "duplicate magic rejected (bare magic before a valid commitment)",
			scriptSig: append(append([]byte{}, magic...), commit...),
			want:      -1,
		},
		{
			name:      "magic too close to end to hold a hash",
			scriptSig: append([]byte{0x00}, magic...), // magic with <32 bytes after
			want:      -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ContainsModelosCommitment(tc.scriptSig, sigma)
			if got != tc.want {
				t.Fatalf("ContainsModelosCommitment = %d, want %d", got, tc.want)
			}
		})
	}
}

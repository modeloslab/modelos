//go:build !zkpow

package zkpow

import (
	"fmt"

	"github.com/modelos/modelos/node/wire"
)

const (
	DefaultNBits     = 0x1E01FFFF
	DefaultM         = 256
	DefaultN         = 512
	DefaultNoiseRank = 32
	DefaultMMAType   = 0
)

func VerifyCertificate(header *wire.BlockHeader, cert wire.BlockCertificate) error {
	return fmt.Errorf("zkpow: build with -tags zkpow to enable proof verification")
}

func VerifyZKCertificateWithNbits(header *wire.BlockHeader, c *wire.ZKCertificate, nbitsOverride uint32) error {
	return fmt.Errorf("zkpow: build with -tags zkpow to enable proof verification with nbits override")
}

func Mine(header *wire.BlockHeader) (*wire.ZKCertificate, error) {
	return nil, fmt.Errorf("zkpow: build with -tags zkpow to enable mining")
}

// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netparams

import (
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/wire"
)

// Params is used to group parameters for various networks such as the main
// network and test networks.
type Params struct {
	*chaincfg.Params
	RPCClientPort string
	RPCServerPort string
}

// MainNetParams contains parameters specific running Oyster and
// pearld on the main network (wire.MainNet).
var MainNetParams = Params{
	Params:        &chaincfg.MainNetParams,
	RPCClientPort: "44107",
	RPCServerPort: "44207",
}

// TestNetParams contains parameters specific running Oyster and
// pearld on the test network (wire.TestNet).
var TestNetParams = Params{
	Params:        &chaincfg.TestNetParams,
	RPCClientPort: "44109",
	RPCServerPort: "44209",
}

// TestNet2Params contains parameters specific running Oyster and
// pearld on the test network v2 (wire.TestNet2).
var TestNet2Params = Params{
	Params:        &chaincfg.TestNet2Params,
	RPCClientPort: "44111",
	RPCServerPort: "44211",
}

// SimNetParams contains parameters specific to the simulation test network
// (wire.SimNet).
var SimNetParams = Params{
	Params:        &chaincfg.SimNetParams,
	RPCClientPort: "18556",
	RPCServerPort: "18554",
}

// SigNetParams contains parameters specific to the signet test network
// (wire.SigNet).
var SigNetParams = Params{
	Params:        &chaincfg.SigNetParams,
	RPCClientPort: "38334",
	RPCServerPort: "38332",
}

// SigNetWire is a helper function that either returns the given chain
// parameter's net value if the parameter represents a signet network or 0 if
// it's not. This is necessary because there can be custom signet networks that
// have a different net value.
func SigNetWire(params *chaincfg.Params) wire.PearlNet {
	if params.Name == chaincfg.SigNetParams.Name {
		return params.Net
	}

	return 0
}

// RegressionNetParams contains parameters specific to the regression test
// network (wire.RegressionNet).
var RegressionNetParams = Params{
	Params:        &chaincfg.RegressionNetParams,
	RPCClientPort: "18334",
	RPCServerPort: "18332",
}

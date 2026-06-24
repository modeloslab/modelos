// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "github.com/modelos/modelos/node/chaincfg"

// activeNetParams is a pointer to the parameters specific to the
// currently active Pearl network.
var activeNetParams = &mainNetParams

// params is used to group parameters for various networks such as the main
// network and test networks.
type params struct {
	*chaincfg.Params
	rpcPort string
}

// mainNetParams contains parameters specific to the main network
// (wire.MainNet).  NOTE: The RPC port is intentionally different from the
// reference implementation because pearld does not handle wallet requests.  The
// separate wallet process listens on the well-known port and forwards requests
// it does not handle on to pearld.  This approach allows the wallet process
// to emulate the full reference implementation RPC API.
var mainNetParams = params{
	Params:  &chaincfg.MainNetParams,
	rpcPort: "44107",
}

// regressionNetParams contains parameters specific to the regression test
// network (wire.RegTest).  NOTE: The RPC port is intentionally different
// than the reference implementation - see the mainNetParams comment for
// details.
var regressionNetParams = params{
	Params:  &chaincfg.RegressionNetParams,
	rpcPort: "18334",
}

// testNetParams contains parameters specific to the test network
// (wire.TestNet).  NOTE: The RPC port is intentionally different from the
// reference implementation - see the mainNetParams comment for details.
var testNetParams = params{
	Params:  &chaincfg.TestNetParams,
	rpcPort: "44109",
}

// testNet2Params contains parameters specific to the test network v2
// (wire.TestNet2).
var testNet2Params = params{
	Params:  &chaincfg.TestNet2Params,
	rpcPort: "44111",
}

// simNetParams contains parameters specific to the simulation test network
// (wire.SimNet).
var simNetParams = params{
	Params:  &chaincfg.SimNetParams,
	rpcPort: "18556",
}

// sigNetParams contains parameters specific to the Signet network
// (wire.SigNet).
var sigNetParams = params{
	Params:  &chaincfg.SigNetParams,
	rpcPort: "38332",
}

// netName returns the name used when referring to a network.
func netName(chainParams *params) string {
	return chainParams.Name
}

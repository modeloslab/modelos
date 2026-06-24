// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package legacyrpc

import (
	"github.com/btcsuite/btclog"
	"github.com/modelos/modelos/wallet/build"
)

var log = btclog.Disabled

func init() {
	UseLogger(build.NewSubLogger("RPCS", nil))
}

// UseLogger sets the package-wide logger.  Any calls to this function must be
// made before a server is created and used (it is not concurrent safe).
func UseLogger(logger btclog.Logger) {
	log = logger
}

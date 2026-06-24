// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"log"

	"github.com/modelos/modelos/node/rpcclient"
)

func main() {
	// Connect to local bitcoin core RPC server using HTTP POST mode over a
	// Unix Socket.
	connCfg := &rpcclient.ConnConfig{
		// For unix sockets, use unix:// + "your unix socket path".
		Host:         "unix:///tmp/test.XXXX",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode.
		DisableTLS:   true, // Bitcoin core does not provide TLS by default.
	}

	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	// Get the current block count.
	blockCount, err := client.GetBlockCount()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Block count: %d", blockCount)
}

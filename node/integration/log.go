//go:build rpctest
// +build rpctest

package integration

import (
	"os"

	"github.com/btcsuite/btclog"
	"github.com/modelos/modelos/node/rpcclient"
)

type logWriter struct{}

func (logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	return len(p), nil
}

func init() {
	backendLog := btclog.NewBackend(logWriter{})
	testLog := backendLog.Logger("ITEST")
	testLog.SetLevel(btclog.LevelDebug)

	rpcclient.UseLogger(testLog)
}

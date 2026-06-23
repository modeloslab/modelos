// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"github.com/modelos/modelos/node/database"
)

// TstRunWithMaxBlockFileSize runs the passed function with the maximum allowed
// file size for the database set to the provided value.  The value will be set
// back to the original value upon completion.
//
// Callers should only use this for testing.
func TstRunWithMaxBlockFileSize(idb database.DB, size uint32, fn func()) {
	ffldb := idb.(*db)
	origSize := ffldb.store.maxBlockFileSize

	ffldb.store.maxBlockFileSize = size
	fn()
	ffldb.store.maxBlockFileSize = origSize
}

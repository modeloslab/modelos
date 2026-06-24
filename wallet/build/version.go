// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package build

import (
	pearlversion "github.com/modelos/modelos/version"
)

// Version returns the application version as a properly formed string per the
// semantic versioning 2.0.0 spec (http://semver.org/).
func Version() string {
	return pearlversion.Version()
}

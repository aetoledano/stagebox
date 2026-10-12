//go:build windows

package controller

import _ "embed"

// Retain the upstream database license without embedding or extracting a
// separate mapping database. All external mappings live beside the executable.
//
//go:embed sdl/gamecontrollerdb-LICENSE.txt
var sdlDatabaseLicense []byte

// Package dlrelay holds module-wide embedded assets.
//
// The root-level package exists only so //go:embed can reach the top-level
// extension/ directory (embed is scoped to the directory of the Go file that
// declares the directive). Runtime code lives under internal/.
package dlrelay

import "embed"

// ExtensionFS contains the browser extension source, embedded at build time
// so release binaries and container images can serve /api/extension.zip
// without shipping the extension directory as a separate asset.
//
//go:embed extension
var ExtensionFS embed.FS

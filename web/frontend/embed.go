// Package frontend embeds the built Web Console assets so the agent-gogo
// binary can serve the UI regardless of the working directory it is run from.
package frontend

import (
	"embed"
	"io/fs"
)

// Dist holds the contents of web/frontend/dist at build time. The on-disk
// dist directory still takes precedence when present (dev rebuilds without
// recompiling); this is the fallback for installed binaries.
//
//go:embed dist
var Dist embed.FS

// DistFS returns the dist subtree as an fs.FS rooted at the dist directory.
func DistFS() fs.FS {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		return Dist
	}
	return sub
}

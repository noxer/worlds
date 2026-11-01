// Package assets embeds all game content: Kenney art, Tiled maps and
// tilesets, and dialog files. Embedding keeps desktop and browser builds
// identical and self-contained.
package assets

import "embed"

//go:embed world.yaml tiles tilesets maps dialog
var FS embed.FS

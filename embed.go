// Package parley embeds the product assets shared by the Go hosts: prompts,
// Domain Packs, presets/voices data, the bundled samples and the vanilla web
// client. Tests and the Server Host read them through Assets.
package parley

import "embed"

//go:embed config packs prompts samples web
var Assets embed.FS

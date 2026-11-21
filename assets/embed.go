// Package assets embeds the bundled themes, plugins and catalog.
package assets

import "embed"

//go:embed catalog.json themes plugins locales
var FS embed.FS

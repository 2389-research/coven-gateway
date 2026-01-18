// ABOUTME: Embeds HTML templates into the binary using go:embed
// ABOUTME: Provides templateFS for loading templates at runtime

package webadmin

import "embed"

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

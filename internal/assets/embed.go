// Package assets serves frontend assets built by Vite and embedded via go:embed.
// It reads the Vite manifest to map entry points to hashed filenames and provides
// a file server with appropriate cache headers.
package assets

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// ManifestEntry represents a single entry in Vite's manifest.json.
type ManifestEntry struct {
	File    string   `json:"file"`
	Src     string   `json:"src,omitempty"`
	IsEntry bool     `json:"isEntry,omitempty"`
	CSS     []string `json:"css,omitempty"`
	Imports []string `json:"imports,omitempty"`
}

// Manifest maps Vite source paths (e.g. "src/islands/auto.ts") to their build outputs.
// Nil when running in dev mode (no manifest available).
// NOTE: Exported and mutable for testability. Not safe for concurrent mutation;
// tests that modify this must not use t.Parallel().
var Manifest map[string]ManifestEntry

// hashPattern detects Vite's content hashes in filenames (e.g. ".CU4W1PlC.").
// Vite uses base64url hashes, so we accept [a-zA-Z0-9_-]. The 8-char minimum
// matches Vite's default hash length. Could false-positive on long words like
// "production" — acceptable since all served files come from Vite output.
// TODO(Phase 2): Consider capping at {8,16} to reduce false positives.
var hashPattern = regexp.MustCompile(`\.[a-zA-Z0-9_-]{8,}\.`)

func init() {
	// Register MIME types that may not be in the default database.
	// Errors are ignored: these only fail if extension format is invalid,
	// and our literals are known-good.
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	_ = mime.AddExtensionType(".map", "application/json")

	data, err := fs.ReadFile(distFS, "dist/.vite/manifest.json")
	if err != nil {
		slog.Debug("no vite manifest found (dev mode?)", "error", err)
		return
	}
	if err := json.Unmarshal(data, &Manifest); err != nil {
		slog.Error("failed to parse vite manifest", "error", err)
	}
}

// containsHash reports whether the given path contains a content hash
// (8+ hex characters between dots, e.g. "auto.a1b2c3d4.js").
func containsHash(p string) bool {
	return hashPattern.MatchString(p)
}

// mimeFromExt returns the MIME type for a file extension.
// Falls back to the Go standard library's MIME type database,
// then to "application/octet-stream" if unknown.
func mimeFromExt(ext string) string {
	switch ext {
	case ".js", ".mjs":
		return "application/javascript"
	case ".css":
		return "text/css; charset=utf-8"
	case ".woff2":
		return "font/woff2"
	case ".svg":
		return "image/svg+xml"
	case ".map":
		return "application/json"
	default:
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
		return "application/octet-stream"
	}
}

const viteDevURL = "http://localhost:5173"

// ScriptTags generates HTML tags for a Vite entry point.
// In production (manifest present): emits stylesheet links, modulepreload hints
// for the full import graph, and a module script tag.
// In dev mode (manifest absent): emits the Vite HMR client and a direct module URL.
func ScriptTags(entry string) string {
	if Manifest == nil {
		return devScriptTags(entry)
	}
	return prodScriptTags(entry)
}

func prodScriptTags(entry string) string {
	e, ok := Manifest[entry]
	if !ok {
		return ""
	}

	var b strings.Builder

	// CSS stylesheets
	for _, css := range e.CSS {
		b.WriteString(`<link rel="stylesheet" href="/static/`)
		b.WriteString(css)
		b.WriteString("\">\n")
	}

	// Modulepreload for the full import graph (prevents waterfall).
	// Sorted for deterministic HTML output across requests.
	seen := make(map[string]bool)
	collectImports(entry, seen)
	imports := make([]string, 0, len(seen))
	for imp := range seen {
		imports = append(imports, imp)
	}
	sort.Strings(imports)
	for _, imp := range imports {
		if me, ok := Manifest[imp]; ok {
			b.WriteString(`<link rel="modulepreload" href="/static/`)
			b.WriteString(me.File)
			b.WriteString("\">\n")
		}
	}

	// Main entry script
	b.WriteString(`<script type="module" src="/static/`)
	b.WriteString(e.File)
	b.WriteString("\"></script>\n")

	return b.String()
}

// collectImports recursively walks the manifest import graph, adding each
// imported entry key to seen. Handles cycles via the seen map.
func collectImports(entry string, seen map[string]bool) {
	e, ok := Manifest[entry]
	if !ok {
		return
	}
	for _, imp := range e.Imports {
		if !seen[imp] {
			seen[imp] = true
			collectImports(imp, seen)
		}
	}
}

func devScriptTags(entry string) string {
	var b strings.Builder
	b.WriteString(`<script type="module" src="`)
	b.WriteString(viteDevURL)
	b.WriteString(`/@vite/client"></script>`)
	b.WriteString("\n")
	b.WriteString(`<script type="module" src="`)
	b.WriteString(viteDevURL)
	b.WriteString("/")
	b.WriteString(entry)
	b.WriteString("\"></script>\n")
	return b.String()
}

// FileServer returns an http.Handler that serves embedded assets from dist/.
// Hashed assets get immutable cache headers; unhashed assets get no-cache.
// The handler expects paths relative to the dist root (strip /static/ before calling).
func FileServer() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("assets: failed to create sub filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set content type explicitly for known extensions
		ext := strings.ToLower(path.Ext(r.URL.Path))
		if ext != "" {
			w.Header().Set("Content-Type", mimeFromExt(ext))
		}

		// Set cache headers based on whether the filename contains a hash
		if containsHash(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		fileServer.ServeHTTP(w, r)
	})
}

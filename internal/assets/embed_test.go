package assets

import (
	"strings"
	"testing"
)

func TestContainsHash(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"js/auto.a1b2c3d4.js", true},
		{"assets/auto.CU4W1PlC.css", true},
		{"js/chunks/vendor.abcdef0123456789.js", true},
		{"index.html", false},
		{"manifest.json", false},
		{".gitkeep", false},
	}
	for _, tt := range tests {
		if got := containsHash(tt.path); got != tt.want {
			t.Errorf("containsHash(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMimeFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".js", "application/javascript"},
		{".mjs", "application/javascript"},
		{".css", "text/css; charset=utf-8"},
		{".woff2", "font/woff2"},
		{".svg", "image/svg+xml"},
		{".map", "application/json"},
		{".qqqqqq", "application/octet-stream"},
	}
	for _, tt := range tests {
		if got := mimeFromExt(tt.ext); got != tt.want {
			t.Errorf("mimeFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestProdScriptTags(t *testing.T) {
	// Save and restore global state
	orig := Manifest
	defer func() { Manifest = orig }()

	Manifest = map[string]ManifestEntry{
		"src/islands/auto.ts": {
			File:    "js/auto.a1b2c3d4.js",
			IsEntry: true,
			CSS:     []string{"assets/auto.e5f6a7b8.css"},
			Imports: []string{"src/lib/shared.ts"},
		},
		"src/lib/shared.ts": {
			File:    "js/chunks/shared.c9d0e1f2.js",
			Imports: []string{"src/lib/utils.ts"},
		},
		"src/lib/utils.ts": {
			File: "js/chunks/utils.a3b4c5d6.js",
		},
	}

	got := ScriptTags("src/islands/auto.ts")

	// CSS link
	if !strings.Contains(got, `<link rel="stylesheet" href="/static/assets/auto.e5f6a7b8.css">`) {
		t.Error("missing CSS stylesheet link")
	}

	// Modulepreload for shared chunk
	if !strings.Contains(got, `<link rel="modulepreload" href="/static/js/chunks/shared.c9d0e1f2.js">`) {
		t.Error("missing modulepreload for shared chunk")
	}

	// Modulepreload for utils chunk (transitive import)
	if !strings.Contains(got, `<link rel="modulepreload" href="/static/js/chunks/utils.a3b4c5d6.js">`) {
		t.Error("missing modulepreload for transitive import (utils)")
	}

	// Main script tag
	if !strings.Contains(got, `<script type="module" src="/static/js/auto.a1b2c3d4.js"></script>`) {
		t.Error("missing main script tag")
	}

	// CSS should come before script
	cssIdx := strings.Index(got, "stylesheet")
	scriptIdx := strings.Index(got, `<script`)
	if cssIdx > scriptIdx {
		t.Error("CSS link should appear before script tag")
	}
}

func TestProdScriptTagsMissingEntry(t *testing.T) {
	orig := Manifest
	defer func() { Manifest = orig }()

	Manifest = map[string]ManifestEntry{}

	got := ScriptTags("nonexistent.ts")
	if got != "" {
		t.Errorf("expected empty string for missing entry, got %q", got)
	}
}

func TestProdScriptTagsCyclicImports(t *testing.T) {
	orig := Manifest
	defer func() { Manifest = orig }()

	// A imports B, B imports A — should not infinite loop
	Manifest = map[string]ManifestEntry{
		"a.ts": {
			File:    "js/a.11111111.js",
			IsEntry: true,
			Imports: []string{"b.ts"},
		},
		"b.ts": {
			File:    "js/b.22222222.js",
			Imports: []string{"a.ts"},
		},
	}

	got := ScriptTags("a.ts")
	if !strings.Contains(got, "js/a.11111111.js") {
		t.Error("missing main script tag")
	}
	if !strings.Contains(got, `modulepreload`) {
		t.Error("missing modulepreload for import")
	}
}

func TestDevScriptTags(t *testing.T) {
	orig := Manifest
	defer func() { Manifest = orig }()

	Manifest = nil // dev mode

	got := ScriptTags("src/islands/auto.ts")

	// Vite HMR client
	if !strings.Contains(got, `<script type="module" src="http://localhost:5173/@vite/client"></script>`) {
		t.Error("missing Vite HMR client script")
	}

	// Direct module URL
	if !strings.Contains(got, `<script type="module" src="http://localhost:5173/src/islands/auto.ts"></script>`) {
		t.Error("missing direct module URL")
	}
}

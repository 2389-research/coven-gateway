// ABOUTME: Verifies GoReleaser generates a valid Homebrew formula for coven-gateway.
// ABOUTME: Protects the formula description and CLI smoke test from known release failures.
package contract

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGoReleaserHomebrewFormulaMatchesCLI(t *testing.T) {
	contents, err := os.ReadFile("../../.goreleaser.yml")
	if err != nil {
		t.Fatalf("read GoReleaser config: %v", err)
	}

	var config struct {
		Brews []struct {
			Description string `yaml:"description"`
			Test        string `yaml:"test"`
		} `yaml:"brews"`
	}
	if err := yaml.Unmarshal(contents, &config); err != nil {
		t.Fatalf("parse GoReleaser config: %v", err)
	}
	if len(config.Brews) != 1 {
		t.Fatalf("GoReleaser config must define exactly one Homebrew formula, got %d", len(config.Brews))
	}

	formula := config.Brews[0]
	if formula.Description != "Gateway server for AI agent orchestration" {
		t.Errorf("Homebrew description must be concise and pass brew style, got %q", formula.Description)
	}

	wantTest := `assert_match "Usage: coven-gateway <command>", shell_output("#{bin}/coven-gateway 2>&1", 1)`
	if strings.TrimSpace(formula.Test) != wantTest {
		t.Errorf("Homebrew test must assert the CLI's real usage output and exit status, got %q", strings.TrimSpace(formula.Test))
	}
}

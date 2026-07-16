// ABOUTME: Verifies the release workflow builds the frontend before packaging binaries.
// ABOUTME: Protects standalone release archives from embedding development-only assets.
package contract

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

func TestReleaseWorkflowBuildsFrontendBeforeGoReleaser(t *testing.T) {
	contents, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	var workflow struct {
		Jobs map[string]struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatalf("parse release workflow: %v", err)
	}

	release, ok := workflow.Jobs["release"]
	if !ok {
		t.Fatal("release workflow must include a release job")
	}

	findStep := func(name string) int {
		for index, step := range release.Steps {
			if step.Name == name {
				return index
			}
		}
		return -1
	}

	nodeIndex := findStep("Set up Node.js")
	frontendIndex := findStep("Build frontend assets")
	releaseIndex := findStep("Run GoReleaser")

	if nodeIndex == -1 {
		t.Error("release job must include a Set up Node.js step")
	} else {
		nodeStep := release.Steps[nodeIndex]
		if nodeStep.Uses != "actions/setup-node@v4" {
			t.Error("Set up Node.js step must use actions/setup-node@v4")
		}
		if nodeStep.With["node-version"] != "22" {
			t.Error("Set up Node.js step must set node-version to 22")
		}
	}

	if frontendIndex == -1 {
		t.Error("release job must include a Build frontend assets step")
	} else if strings.TrimSpace(release.Steps[frontendIndex].Run) != "make web" {
		t.Error("Build frontend assets step must run exactly make web")
	}

	if releaseIndex == -1 {
		t.Error("release job must include a Run GoReleaser step")
	} else if release.Steps[releaseIndex].Uses != "goreleaser/goreleaser-action@v6" {
		t.Error("Run GoReleaser step must use goreleaser/goreleaser-action@v6")
	}

	if nodeIndex != -1 && frontendIndex != -1 && releaseIndex != -1 &&
		(nodeIndex >= frontendIndex || frontendIndex >= releaseIndex) {
		t.Error("release job must set up Node.js, build frontend assets, then run GoReleaser")
	}
}

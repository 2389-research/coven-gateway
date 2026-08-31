// ABOUTME: Discovers Claude Code sessions running in tmux panes.
// ABOUTME: Uses tmux list-panes + ps to identify panes running claude or claude-sim binaries.

package tmux

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Session represents a discovered Claude Code session in a tmux pane.
type Session struct {
	// SessionName is the tmux session name (e.g. "claude-test-gateway").
	SessionName string
	// PaneID is the tmux pane identifier (e.g. "%0").
	PaneID string
	// PanePID is the process ID of the pane's foreground process.
	PanePID string
	// BinaryPath is the resolved path to the running binary (e.g. "/Users/x/.local/bin/claude").
	BinaryPath string
	// BinaryName is the base name of the binary (e.g. "claude", "claude-sim").
	BinaryName string
	// WorkingDir is the pane's current working directory.
	WorkingDir string
	// IsSimulator is true if this is a claude-sim instance (for testing).
	IsSimulator bool
}

// AgentID returns a stable identifier for this session suitable for coven registration.
func (s Session) AgentID() string {
	return fmt.Sprintf("tmux-%s-%s", s.SessionName, strings.TrimPrefix(s.PaneID, "%"))
}

// DisplayName returns a human-readable name for the agent.
func (s Session) DisplayName() string {
	dir := filepath.Base(s.WorkingDir)
	if s.IsSimulator {
		return "Claude (sim) @ " + dir
	}
	return "Claude @ " + dir
}

// knownBinaries are binary names that indicate a Claude Code session.
var knownBinaries = []string{"claude", "claude-sim"}

// Discoverer finds Claude Code sessions in tmux panes.
type Discoverer struct {
	logger *slog.Logger
}

// NewDiscoverer creates a new tmux session discoverer.
func NewDiscoverer(logger *slog.Logger) *Discoverer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Discoverer{logger: logger}
}

// Discover scans all tmux panes and returns those running Claude Code.
func (d *Discoverer) Discover(ctx context.Context) ([]Session, error) {
	// list-panes -a: all panes across all sessions.
	// Format fields: session_name, pane_id, pane_pid, pane_current_path, pane_dead
	out, err := tmuxCmd(ctx, "list-panes", "-a", "-F",
		"#{session_name}\t#{pane_id}\t#{pane_pid}\t#{pane_current_path}\t#{pane_dead}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	var sessions []Session
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 5 {
			d.logger.Warn("unexpected tmux output", "line", line)
			continue
		}
		sessionName := fields[0]
		paneID := fields[1]
		panePID := fields[2]
		workDir := fields[3]
		dead := fields[4]

		if dead == "1" {
			continue
		}

		// Resolve the actual binary running in this pane.
		// pane_pid is typically the shell (zsh/bash); Claude runs as a child.
		binPath, _ := findClaudeBinary(ctx, panePID)
		if binPath == "" {
			continue
		}

		binName := filepath.Base(binPath)

		sess := Session{
			SessionName: sessionName,
			PaneID:      paneID,
			PanePID:     panePID,
			BinaryPath:  binPath,
			BinaryName:  binName,
			WorkingDir:  workDir,
			IsSimulator: binName == "claude-sim",
		}
		d.logger.Info("discovered Claude session",
			"session", sessionName,
			"pane", paneID,
			"binary", binName,
			"cwd", workDir,
		)
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// findClaudeBinary checks the given PID and its children (up to 2 levels deep)
// for a known Claude binary. Returns (binaryPath, pid, nil) if found, or ("", "", nil)
// if no Claude binary exists in the process tree. The pid return value is the PID
// of the process running the Claude binary (which may differ from the input pid).
func findClaudeBinary(ctx context.Context, pid string) (binPath string, claudePID string) {
	// Check the PID itself first (handles direct exec, e.g. claude-sim).
	if path, ok := resolveIfClaude(ctx, pid); ok {
		return path, pid
	}

	// Walk children — real Claude sessions are typically shell → claude.
	children := childPIDs(ctx, pid)
	for _, cpid := range children {
		if path, ok := resolveIfClaude(ctx, cpid); ok {
			return path, cpid
		}
	}

	// One more level: shell → wrapper → claude (uncommon but possible).
	for _, cpid := range children {
		for _, gpid := range childPIDs(ctx, cpid) {
			if path, ok := resolveIfClaude(ctx, gpid); ok {
				return path, gpid
			}
		}
	}

	return "", ""
}

// resolveIfClaude resolves a PID's binary and returns its path if it's a known
// Claude binary. Returns ("", false) if the PID doesn't resolve or isn't Claude.
func resolveIfClaude(ctx context.Context, pid string) (string, bool) {
	binPath, err := resolveBinary(ctx, pid)
	if err != nil {
		return "", false
	}
	if isClaudeBinary(filepath.Base(binPath)) {
		return binPath, true
	}
	return "", false
}

// resolveBinary gets the binary name/path for a PID.
// On macOS: ps -o comm= -p <pid> (returns full path for Mach-O binaries).
// On Linux: readlink /proc/<pid>/exe.
func resolveBinary(ctx context.Context, pid string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		target, err := exec.CommandContext(ctx, "readlink", "/proc/"+pid+"/exe").Output()
		if err != nil {
			return "", fmt.Errorf("readlink /proc/%s/exe: %w", pid, err)
		}
		return strings.TrimSpace(string(target)), nil
	default: // darwin, freebsd, etc.
		out, err := exec.CommandContext(ctx, "ps", "-o", "comm=", "-p", pid).Output()
		if err != nil {
			return "", fmt.Errorf("ps comm for pid %s: %w", pid, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// childPIDs returns the PIDs of direct child processes. Works on macOS and Linux.
// Returns nil if the process has no children or pgrep fails (both are normal).
func childPIDs(ctx context.Context, pid string) []string {
	out, err := exec.CommandContext(ctx, "pgrep", "-P", pid).Output()
	if err != nil {
		return nil // pgrep exits 1 when no children — not an error
	}
	var pids []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			pids = append(pids, line)
		}
	}
	return pids
}

// isClaudeBinary checks if a binary name matches known Claude Code binaries.
func isClaudeBinary(name string) bool {
	return slices.Contains(knownBinaries, name)
}

// tmuxCmd runs a tmux command and returns its stdout.
func tmuxCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%w: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

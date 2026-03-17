// ABOUTME: coven-tmux-adapter discovers Claude Code sessions in tmux and registers them as coven agents.
// ABOUTME: It auto-discovers tmux panes running claude/claude-sim, bridges terminal I/O to gRPC.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/2389/coven-gateway/internal/tmux"
)

// bridgeEntry tracks a running bridge and its cancel function.
type bridgeEntry struct {
	bridge *tmux.Bridge
	cancel context.CancelFunc
}

func main() {
	gatewayAddr := flag.String("gateway", "localhost:50051", "coven-gateway gRPC address")
	scanInterval := flag.Duration("scan-interval", 10*time.Second, "interval between tmux scans")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	oneShot := flag.Bool("one-shot", false, "discover and register once, then block (no re-scanning)")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, logger, *gatewayAddr, *scanInterval, *oneShot)
	cancel()
	if err != nil {
		logger.Error("adapter exited", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, gatewayAddr string, scanInterval time.Duration, oneShot bool) error {
	discoverer := tmux.NewDiscoverer(logger)

	// Track active bridges by agent ID to avoid duplicates.
	bridges := make(map[string]*bridgeEntry)
	var mu sync.Mutex

	scan := func() {
		sessions, err := discoverer.Discover(ctx)
		if err != nil {
			logger.Error("discovery failed", "err", err)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		// Track which sessions are still alive for cleanup.
		alive := make(map[string]bool)

		for _, sess := range sessions {
			agentID := sess.AgentID()
			alive[agentID] = true

			if _, exists := bridges[agentID]; exists {
				continue // already bridged
			}

			logger.Info("new session discovered, starting bridge",
				"agent_id", agentID,
				"session", sess.SessionName,
				"binary", sess.BinaryName,
				"cwd", sess.WorkingDir,
			)

			bridgeCtx, bridgeCancel := context.WithCancel(ctx)
			bridge := tmux.NewBridge(sess, gatewayAddr, logger)
			bridges[agentID] = &bridgeEntry{bridge: bridge, cancel: bridgeCancel}

			go func(id string) {
				if err := bridge.Run(bridgeCtx); err != nil {
					logger.Error("bridge exited", "agent_id", id, "err", err)
				}
				mu.Lock()
				delete(bridges, id)
				mu.Unlock()
			}(agentID)
		}

		// Clean up bridges for sessions that no longer exist.
		for id, entry := range bridges {
			if !alive[id] {
				logger.Info("session disappeared, stopping bridge", "agent_id", id)
				entry.cancel()
				delete(bridges, id)
			}
		}
	}

	// Initial scan.
	scan()

	mu.Lock()
	count := len(bridges)
	mu.Unlock()
	fmt.Fprintf(os.Stderr, "coven-tmux-adapter: %d session(s) bridged to %s\n", count, gatewayAddr)

	if oneShot {
		<-ctx.Done()
		return nil
	}

	// Continuous re-scanning + periodic status logging.
	scanTicker := time.NewTicker(scanInterval)
	defer scanTicker.Stop()

	statusTicker := time.NewTicker(60 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			mu.Lock()
			for _, entry := range bridges {
				entry.cancel()
			}
			mu.Unlock()
			return nil
		case <-scanTicker.C:
			scan()
		case <-statusTicker.C:
			logBridgeStatus(logger, &mu, bridges)
		}
	}
}

// logBridgeStatus logs the current state of all active bridges.
func logBridgeStatus(logger *slog.Logger, mu *sync.Mutex, bridges map[string]*bridgeEntry) {
	mu.Lock()
	defer mu.Unlock()

	if len(bridges) == 0 {
		logger.Info("adapter status: no active bridges")
		return
	}

	states := make(map[string]int)
	for _, entry := range bridges {
		status := entry.bridge.Status()
		states[status]++
	}

	logger.Info("adapter status",
		"bridges", len(bridges),
		"states", states,
	)
}

// ABOUTME: Utility functions for the tmux adapter — hostname, OS detection.

package tmux

import (
	"os"
	"runtime"
)

func hostName() (string, error) {
	return os.Hostname()
}

func osName() string {
	return runtime.GOOS
}

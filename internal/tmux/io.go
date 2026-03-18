// Package tmux handles terminal I/O for tmux panes — send input via send-keys,
// capture output via pipe-pane. It also strips ANSI sequences, detects prompts,
// and tracks response boundaries.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ANSI escape sequence pattern for stripping terminal formatting.
// Covers: CSI sequences (\x1b[...X), CSI with ? prefix (\x1b[?...X for DEC private modes),
// OSC sequences (\x1b]...\x07), and carriage returns.
// Note: cursor-right (\x1b[nC) and cursor-down (\x1b[nB) are handled separately
// by StripANSI to preserve word spacing and line breaks before this regex runs.
var ansiRegex = regexp.MustCompile(`\x1b\[\??[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\r`)

// cursorRightRegex matches CSI cursor-forward sequences (\x1b[nC).
// Claude's TUI uses these as spaces between words; stripping them loses word boundaries.
var cursorRightRegex = regexp.MustCompile(`\x1b\[(\d*)C`)

// cursorDownRegex matches CSI cursor-down sequences (\x1b[nB).
// Claude's TUI uses these for line advancement; replacing with newlines
// preserves row structure so prompts and rules appear on their own lines.
var cursorDownRegex = regexp.MustCompile(`\x1b\[(\d*)B`)

// promptPattern matches Claude Code's magenta arrow prompt (after ANSI stripping).
// The prompt appears as "❯ " at the start of a line.
var promptPattern = regexp.MustCompile(`(?m)^❯\s*$`)

// horizontalRulePattern matches Claude's dim horizontal rule separator.
// Allows trailing whitespace/TUI chrome after the rule characters.
var horizontalRulePattern = regexp.MustCompile(`(?m)^─{10,}`)

// thinkingPattern matches Claude Code's animated thinking indicator.
// Claude uses verbs like "Thinking...", "Pontificating…", "Channeling…" etc.
// The pattern looks for a word ending with "…" or "..." optionally preceded by
// a spinner character (✽, ✻, ✶, ✳, ✢, ·, ⏺, or braille spinners).
var thinkingPattern = regexp.MustCompile(`(?:Thinking\.\.\.|[\p{L}]+…)`)

// hasRealContent checks if a line has substantive text, not a TUI rendering fragment.
// Requires a space (multi-token) AND at least one token of 4+ characters.
// Single characters/word fragments from character-at-a-time rendering ("P", "ing",
// "Hello") are filtered; real sentences ("Answer 1.", "The answer is 42") pass.
func hasRealContent(s string) bool {
	if !strings.ContainsRune(s, ' ') {
		return false // single token — likely a rendering fragment
	}
	for word := range strings.SplitSeq(s, " ") {
		if len(strings.TrimSpace(word)) >= 4 {
			return true
		}
	}
	return false
}

// tuiChromePatterns matches Claude Code's TUI chrome that gets mixed into pipe-pane output.
// These appear on the same line as response text due to terminal re-rendering.
var tuiChromePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)How is Claude doing this session\?.*`),
	regexp.MustCompile(`\d+:\s*Bad.*\d+:\s*Fine.*\d+:\s*Good.*\d+:\s*Dismiss`),
	regexp.MustCompile(`bypass permissions on.*shift\+tab.*`),
	regexp.MustCompile(`[⏵]{2}.*`), // double-triangle permission hint prefix
}

// cleanTUIChrome removes Claude's TUI chrome from a line (feedback prompts, permission hints).
func cleanTUIChrome(line string) string {
	// Find the earliest chrome match and truncate there.
	// TUI chrome always appears at the end of actual content.
	earliest := len(line)
	for _, pat := range tuiChromePatterns {
		if loc := pat.FindStringIndex(line); loc != nil && loc[0] < earliest {
			earliest = loc[0]
		}
	}
	if earliest < len(line) {
		line = line[:earliest]
	}
	return line
}

// PaneIO handles input/output for a single tmux pane.
type PaneIO struct {
	paneID string
	logger *slog.Logger

	// pipe-pane output state
	pipeMu   sync.Mutex
	pipeFile *os.File
	pipePath string
}

// NewPaneIO creates I/O handler for the given tmux pane.
func NewPaneIO(paneID string, logger *slog.Logger) *PaneIO {
	if logger == nil {
		logger = slog.Default()
	}
	return &PaneIO{
		paneID: paneID,
		logger: logger.With("pane", paneID),
	}
}

// SendInput sends text to the tmux pane via send-keys.
// The text is sent literally (-l flag) to avoid key binding interpretation.
// An Enter key is appended to submit the input.
func (p *PaneIO) SendInput(ctx context.Context, text string) error {
	// send-keys -l sends literal text (no key binding interpretation).
	// We send text + Enter as separate commands to ensure proper submission.
	if _, err := tmuxCmd(ctx, "send-keys", "-t", p.paneID, "-l", text); err != nil {
		return fmt.Errorf("send-keys text: %w", err)
	}
	if _, err := tmuxCmd(ctx, "send-keys", "-t", p.paneID, "Enter"); err != nil {
		return fmt.Errorf("send-keys Enter: %w", err)
	}
	return nil
}

// CapturePane captures the full scrollback + visible content of the pane.
// Uses -S - -E - to get complete history (without this, scrollback is silently truncated).
func (p *PaneIO) CapturePane(ctx context.Context) (string, error) {
	out, err := tmuxCmd(ctx, "capture-pane", "-p", "-t", p.paneID, "-S", "-", "-E", "-")
	if err != nil {
		return "", fmt.Errorf("capture-pane: %w", err)
	}
	return out, nil
}

// StartPipePane starts piping the pane's output to a temp file for real-time capture.
// Returns the path to the pipe file. Call StopPipePane to clean up.
func (p *PaneIO) StartPipePane(ctx context.Context) (string, error) {
	p.pipeMu.Lock()
	defer p.pipeMu.Unlock()

	if p.pipeFile != nil {
		return p.pipePath, nil // already running
	}

	f, err := os.CreateTemp("", "coven-pipe-"+strings.TrimPrefix(p.paneID, "%")+"-*")
	if err != nil {
		return "", fmt.Errorf("create pipe temp file: %w", err)
	}
	p.pipeFile = f
	p.pipePath = f.Name()

	// Clear any orphaned pipe-pane first (from previous adapter runs killed with SIGTERM).
	_, _ = tmuxCmd(ctx, "pipe-pane", "-t", p.paneID)

	// pipe-pane -o: output only (don't capture input typed by user).
	// Pipes raw terminal output including ANSI sequences.
	if _, err := tmuxCmd(ctx, "pipe-pane", "-o", "-t", p.paneID,
		"cat >> "+p.pipePath); err != nil {
		_ = f.Close()
		_ = os.Remove(p.pipePath)
		p.pipeFile = nil
		p.pipePath = ""
		return "", fmt.Errorf("pipe-pane: %w", err)
	}

	// Verify the pipe is actually active.
	pipeStatus, _ := tmuxCmd(ctx, "display-message", "-p", "-t", p.paneID, "#{pane_pipe}")
	pipeStatus = strings.TrimSpace(pipeStatus)
	p.logger.Debug("pipe-pane started", "path", p.pipePath, "pane_pipe", pipeStatus)
	if pipeStatus != "1" {
		p.logger.Warn("pipe-pane NOT active after setup!", "pane_pipe", pipeStatus)
	}
	return p.pipePath, nil
}

// StopPipePane stops the pipe-pane capture and cleans up.
func (p *PaneIO) StopPipePane(ctx context.Context) error {
	p.pipeMu.Lock()
	defer p.pipeMu.Unlock()

	if p.pipeFile == nil {
		return nil
	}

	p.logger.Debug("stopping pipe-pane", "path", p.pipePath)

	// Empty string disables pipe-pane.
	if _, err := tmuxCmd(ctx, "pipe-pane", "-t", p.paneID); err != nil {
		p.logger.Warn("failed to stop pipe-pane", "err", err)
	}

	_ = p.pipeFile.Close()
	_ = os.Remove(p.pipePath)
	p.pipeFile = nil
	p.pipePath = ""
	return nil
}

// CurrentPipeSize returns the current size of the pipe file.
// Use this before sending input to get the starting offset for WaitForResponse.
func (p *PaneIO) CurrentPipeSize() int64 {
	p.pipeMu.Lock()
	path := p.pipePath
	p.pipeMu.Unlock()

	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ReadPipeOutput reads all new content from the pipe file since the given offset.
// Returns the content and the new offset.
func (p *PaneIO) ReadPipeOutput(offset int64) (string, int64, error) {
	p.pipeMu.Lock()
	path := p.pipePath
	p.pipeMu.Unlock()

	if path == "" {
		return "", offset, errors.New("pipe-pane not started")
	}

	f, err := os.Open(path) //nolint:gosec // G304: path is a controlled temp file created by StartPipePane
	if err != nil {
		return "", offset, fmt.Errorf("open pipe file: %w", err)
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		return "", offset, fmt.Errorf("seek pipe file: %w", err)
	}

	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return "", offset, fmt.Errorf("read pipe file: %w", err)
	}

	newOffset := offset + int64(len(data))
	return string(data), newOffset, nil
}

// StripANSI removes ANSI escape sequences from terminal output.
// Cursor-right (\x1b[nC) is replaced with spaces and cursor-down (\x1b[nB)
// with newlines BEFORE stripping, so word boundaries and line breaks survive.
func StripANSI(s string) string {
	// Phase 1: Replace cursor movements with whitespace equivalents.
	s = cursorRightRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", parseCursorN(m))
	})
	s = cursorDownRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat("\n", parseCursorN(m))
	})
	// Phase 2: Strip remaining ANSI sequences and carriage returns.
	return ansiRegex.ReplaceAllString(s, "")
}

// parseCursorN extracts N from a CSI sequence like \x1b[NC or \x1b[NB.
// Returns 1 when N is omitted (CSI default).
func parseCursorN(seq string) int {
	// seq is \x1b[<digits><letter>, extract the digits.
	inner := seq[2 : len(seq)-1]
	if inner == "" {
		return 1
	}
	if n, err := strconv.Atoi(inner); err == nil && n > 0 {
		return min(n, 200) // cap to avoid absurd allocations
	}
	return 1
}

// ResponseState tracks the state of Claude's response in terminal output.
type ResponseState int

const (
	// StateIdle means Claude is at the prompt, waiting for input.
	StateIdle ResponseState = iota
	// StateThinking means Claude is showing the thinking spinner.
	StateThinking
	// StateResponding means Claude is streaming response text.
	StateResponding
	// StateDone means Claude has finished responding (horizontal rule + prompt detected).
	StateDone
)

// ResponseTracker monitors terminal output to detect response boundaries.
type ResponseTracker struct {
	state         ResponseState
	mu            sync.Mutex
	textBuf       strings.Builder
	pending       string // incomplete line waiting for \n
	inputText     string // the input text sent, used to filter echo
	logger        *slog.Logger
	onChange      func(state ResponseState, text string)
	sawRule       bool // saw horizontal rule, next prompt means done
	doneCandidate bool // saw rule+prompt, waiting to confirm it's not TUI input area
}

// NewResponseTracker creates a tracker that calls onChange when state transitions occur.
// inputText is the message that was sent — lines matching it will be filtered out.
func NewResponseTracker(logger *slog.Logger, inputText string, onChange func(ResponseState, string)) *ResponseTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ResponseTracker{
		state:     StateIdle,
		inputText: strings.TrimSpace(inputText),
		logger:    logger,
		onChange:  onChange,
	}
}

// Feed processes new terminal output (already ANSI-stripped) and updates state.
func (r *ResponseTracker) Feed(clean string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Prepend any incomplete line from the previous chunk.
	data := r.pending + clean
	r.pending = ""

	// Split into lines. If data doesn't end with \n, the last element is
	// an incomplete line that we carry over to the next Feed() call.
	lines := strings.Split(data, "\n")
	if !strings.HasSuffix(data, "\n") && len(lines) > 0 {
		r.pending = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}

	for _, line := range lines {
		r.processLine(line)
	}
}

// processLine handles a single complete line of terminal output.
// Must be called with r.mu held.
func (r *ResponseTracker) processLine(line string) {
	trimmed := strings.TrimSpace(line)

	if r.shouldSkipLine(trimmed) {
		return
	}

	// In StateIdle, detect thinking BEFORE the spinner filter.
	// Claude shows animated verbs: "Thinking...", "Pontificating…", "Channeling…", etc.
	if r.state == StateIdle && thinkingPattern.MatchString(trimmed) {
		r.transitionToThinking()
		return
	}

	if isSpinnerFrame(trimmed) {
		return
	}

	switch r.state {
	case StateThinking:
		r.handleThinkingLine(line, trimmed)
	case StateResponding:
		r.handleRespondingLine(line, trimmed)
	case StateDone:
		r.state = StateIdle
	}
}

// shouldSkipLine returns true for lines that should be ignored in all states.
// Must be called with r.mu held.
func (r *ResponseTracker) shouldSkipLine(trimmed string) bool {
	if trimmed == "" && r.state != StateResponding {
		return true
	}
	if r.inputText != "" && r.isInputEcho(trimmed) {
		return true
	}
	// Skip lines that are entirely TUI chrome (feedback prompt, permission hint).
	if isTUIChromeLine(trimmed) {
		return true
	}
	return false
}

// isTUIChromeLine returns true if the line consists entirely of TUI chrome.
func isTUIChromeLine(trimmed string) bool {
	for _, pat := range tuiChromePatterns {
		if pat.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// transitionToThinking moves from idle to thinking state.
// Must be called with r.mu held.
func (r *ResponseTracker) transitionToThinking() {
	r.state = StateThinking
	r.sawRule = false
	r.doneCandidate = false
	r.logger.Debug("state: idle → thinking")
	r.notify(StateThinking, "")
}

// handleThinkingLine processes a line while in the thinking state.
// Must be called with r.mu held.
func (r *ResponseTracker) handleThinkingLine(line, trimmed string) {
	if thinkingPattern.MatchString(trimmed) {
		return // still thinking
	}
	if isSpinnerFrame(trimmed) || trimmed == "" {
		return
	}
	// Horizontal rules during thinking are TUI separators (between prompt and response area).
	if horizontalRulePattern.MatchString(trimmed) {
		return
	}
	// Prompt lines (❯) appear in TUI re-renders during thinking — skip them.
	if promptPattern.MatchString(trimmed) {
		return
	}
	// Any real content means response started.
	cleaned := cleanTUIChrome(line)
	cleanedTrimmed := strings.TrimSpace(cleaned)
	if cleanedTrimmed == "" {
		return // was only TUI chrome
	}
	// Skip TUI rendering fragments (single characters from incremental re-draws).
	// Real response text always contains at least two words of 3+ characters.
	if !hasRealContent(cleanedTrimmed) {
		return
	}
	r.state = StateResponding
	r.textBuf.Reset()
	r.sawRule = false
	r.textBuf.WriteString(cleaned)
	r.textBuf.WriteString("\n")
	r.logger.Debug("state: thinking → responding")
	r.notify(StateResponding, cleaned)
}

// handleRespondingLine processes a line while in the responding state.
// Must be called with r.mu held.
func (r *ResponseTracker) handleRespondingLine(line, trimmed string) {
	// Check pending done candidate first — may cancel or confirm it.
	if r.doneCandidate && r.resolveDoneCandidate(trimmed) {
		return
	}
	// Boundary detection: rules, prompts, and thinking indicators.
	if r.handleRespondingBoundary(trimmed) {
		return
	}
	// Clean TUI chrome from response lines.
	cleaned := cleanTUIChrome(line)
	cleanedTrimmed := strings.TrimSpace(cleaned)
	if cleanedTrimmed == "" {
		return // was only TUI chrome
	}
	r.textBuf.WriteString(cleaned)
	r.textBuf.WriteString("\n")
	r.notify(StateResponding, cleaned)
}

// handleRespondingBoundary checks for boundary markers (rules, prompts, thinking)
// during the responding state. Returns true if the line was consumed.
// Must be called with r.mu held.
func (r *ResponseTracker) handleRespondingBoundary(trimmed string) bool {
	if horizontalRulePattern.MatchString(trimmed) {
		r.sawRule = true
		return true
	}
	// Prompt after rule → set done candidate (deferred, not immediate).
	// TUI re-renders include rule+prompt as input area chrome between response lines.
	if promptPattern.MatchString(trimmed) {
		if r.sawRule {
			r.doneCandidate = true
			r.sawRule = false
			r.logger.Debug("done candidate set (rule+prompt)")
		}
		return true
	}
	// Still-thinking lines (Claude re-renders thinking indicator mid-response).
	if thinkingPattern.MatchString(trimmed) && !hasRealContent(trimmed) {
		return true
	}
	// After a rule, empty lines before the prompt are expected.
	if r.sawRule {
		if trimmed == "" {
			return true // gap between rule and prompt
		}
		r.sawRule = false // non-prompt text after rule = false boundary
	}
	return false
}

// resolveDoneCandidate checks a line that follows a rule+prompt "done candidate".
// Returns true if the line was consumed (caller should return), false if the line
// should be processed normally (done candidate was cancelled, content follows).
// Claude's TUI input area is: rule → prompt → rule → bypass. The second rule
// and bypass are expected chrome — skip them but keep the done candidate alive.
// Only real response content cancels the candidate (false boundary).
// Must be called with r.mu held.
func (r *ResponseTracker) resolveDoneCandidate(trimmed string) bool {
	if horizontalRulePattern.MatchString(trimmed) {
		// Second rule is expected (TUI input area bottom border). Skip it
		// but keep doneCandidate alive — done is confirmed by FlushPending
		// when no more data arrives.
		return true
	}
	if trimmed == "" || promptPattern.MatchString(trimmed) {
		return true // gap or redundant prompt while waiting to confirm
	}
	// Real content after rule+prompt → false boundary, continue responding.
	r.doneCandidate = false
	r.sawRule = false
	r.logger.Debug("done candidate cancelled: more content follows")
	return false
}

// completeDone transitions from responding to done, then resets to idle.
// Must be called with r.mu held.
func (r *ResponseTracker) completeDone() {
	r.state = StateDone
	text := r.textBuf.String()
	r.logger.Debug("state: responding → done", "len", len(text))
	r.notify(StateDone, text)
	r.state = StateIdle
	r.textBuf.Reset()
	r.sawRule = false
	r.doneCandidate = false
}

// notify calls the onChange callback if set.
// Must be called with r.mu held.
func (r *ResponseTracker) notify(state ResponseState, text string) {
	if r.onChange != nil {
		r.onChange(state, text)
	}
}

// FlushPending processes any pending (incomplete line) data for state-detection.
// Call this after Feed() to handle the case where the final output (e.g. prompt "❯")
// doesn't end with \n and would otherwise be stuck in the pending buffer.
// It also completes deferred done detection when no more data follows a rule+prompt.
func (r *ResponseTracker) FlushPending() {
	r.mu.Lock()
	defer r.mu.Unlock()

	trimmed := ""
	if r.pending != "" {
		trimmed = strings.TrimSpace(r.pending)
	}

	// Only process pending for state transitions, not text accumulation.
	// This avoids fragmenting partial words into the text buffer.
	switch r.state {
	case StateIdle:
		if trimmed != "" && thinkingPattern.MatchString(trimmed) {
			r.pending = ""
			r.state = StateThinking
			r.sawRule = false
			r.logger.Debug("state: idle → thinking (from pending)")
			r.notify(StateThinking, "")
		}
	case StateResponding:
		if trimmed != "" {
			r.flushPendingResponding(trimmed)
		}
		// If we saw rule+prompt and no more data followed, the response is done.
		if r.doneCandidate {
			r.pending = ""
			r.completeDone()
		}
	}
}

// flushPendingResponding handles pending data while in the responding state.
// Must be called with r.mu held.
func (r *ResponseTracker) flushPendingResponding(trimmed string) {
	// If pending is a second rule after a done candidate, skip it
	// (expected TUI input area bottom border). Keep doneCandidate alive.
	if r.doneCandidate && horizontalRulePattern.MatchString(trimmed) {
		r.pending = ""
		return
	}
	if horizontalRulePattern.MatchString(trimmed) {
		r.sawRule = true
		r.pending = ""
		r.logger.Debug("horizontal rule detected (from pending)")
		return
	}
	if r.sawRule && promptPattern.MatchString(trimmed) {
		r.pending = ""
		// Defer done: set candidate instead of completing immediately.
		r.doneCandidate = true
		r.sawRule = false
		r.logger.Debug("done candidate set from pending (rule+prompt)")
	}
}

// isInputEcho checks if a line is the echoed input (with or without prompt prefix).
func (r *ResponseTracker) isInputEcho(trimmed string) bool {
	// Exact match.
	if trimmed == r.inputText {
		return true
	}
	// Prompt + input (e.g. "❯ explain gRPC").
	if strings.HasSuffix(trimmed, r.inputText) && strings.Contains(trimmed, "❯") {
		return true
	}
	return false
}

// State returns the current response state (for external status checks).
func (r *ResponseTracker) State() ResponseState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// String returns a human-readable name for a ResponseState.
func (s ResponseState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateThinking:
		return "thinking"
	case StateResponding:
		return "responding"
	case StateDone:
		return "done"
	default:
		return "unknown"
	}
}

// WaitForResponse blocks until a complete response is captured or the context expires.
// startOffset should be PaneIO.CurrentPipeSize() captured BEFORE sending input.
// installResponseCallbacks resets the tracker and wires up channels for done/thinking signals.
func installResponseCallbacks(tracker *ResponseTracker, result *string, done, thinkingDetected chan struct{}) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	origOnChange := tracker.onChange
	tracker.state = StateIdle
	tracker.textBuf.Reset()
	tracker.pending = ""
	tracker.sawRule = false
	tracker.doneCandidate = false
	tracker.onChange = func(state ResponseState, text string) {
		if origOnChange != nil {
			origOnChange(state, text)
		}
		switch state {
		case StateThinking:
			select {
			case thinkingDetected <- struct{}{}:
			default:
			}
		case StateDone:
			*result = text
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}
}

// drainTimer stops a timer and drains its channel if needed.
func drainTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// pollPipeOutput reads new pipe data, feeds it to the tracker, and returns the new offset.
// FlushPending is called on every poll (not just when new data arrives) so that
// deferred done detection completes when the pipe stops producing output.
func pollPipeOutput(tracker *ResponseTracker, paneIO *PaneIO, offset int64) int64 {
	raw, newOffset, err := paneIO.ReadPipeOutput(offset)
	if err != nil {
		tracker.logger.Debug("read pipe error", "err", err, "offset", offset)
		return offset
	}
	if newOffset > offset {
		tracker.logger.Debug("pipe data read", "bytes", newOffset-offset, "offset", offset)
		clean := StripANSI(raw)
		tracker.Feed(clean)
	}
	tracker.FlushPending()
	return newOffset
}

func WaitForResponse(ctx context.Context, tracker *ResponseTracker, paneIO *PaneIO, startOffset int64, pollInterval time.Duration) (string, error) {
	var result string
	done := make(chan struct{}, 1)
	thinkingDetected := make(chan struct{}, 1)

	installResponseCallbacks(tracker, &result, done, thinkingDetected)

	offset := startOffset
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Busy detection: if Claude doesn't start thinking within 30s,
	// it's likely already processing something else.
	const busyTimeout = 30 * time.Second
	busyTimer := time.NewTimer(busyTimeout)
	defer busyTimer.Stop()
	busyChecked := false

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-done:
			return strings.TrimSpace(result), nil
		case <-thinkingDetected:
			drainTimer(busyTimer)
			busyChecked = true
		case <-busyTimer.C:
			if !busyChecked {
				return "", errors.New("claude appears busy: no thinking detected within 30s (may be processing another request)")
			}
		case <-ticker.C:
			offset = pollPipeOutput(tracker, paneIO, offset)
		}
	}
}

func isSpinnerFrame(s string) bool {
	// Braille spinners (claude-sim and older Claude versions).
	// Star/dot spinners (Claude Code v2.x TUI).
	spinners := []string{
		"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		"✽", "✻", "✶", "✳", "✢", "·", "⏺",
	}
	for _, sp := range spinners {
		if strings.HasPrefix(s, sp) {
			return true
		}
	}
	return false
}

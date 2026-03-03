package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── ANSI Color Codes ──

const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Foreground colors.
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bright foreground.
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	// Background colors.
	BgBlue    = "\033[44m"
	BgCyan    = "\033[46m"
	BgMagenta = "\033[45m"

	// Cursor control.
	ClearLine = "\033[2K"
	MoveUp    = "\033[1A"
)

// ── Theme ──

var (
	ColorPrompt    = BrightCyan
	ColorAgent     = BrightMagenta
	ColorSuccess   = BrightGreen
	ColorError     = BrightRed
	ColorWarning   = BrightYellow
	ColorInfo      = BrightBlue
	ColorDim       = BrightBlack
	ColorResponse  = BrightGreen
	ColorAccent    = BrightCyan
	ColorHighlight = Bold + BrightWhite
)

// ── Icons ──

const (
	IconPrompt    = "❯"
	IconThinking  = "◆"
	IconDelegate  = "→"
	IconSuccess   = "✓"
	IconError     = "✗"
	IconWarning   = "⚠"
	IconInfo      = "●"
	IconSynth     = "◇"
	IconAgent     = "▸"
	IconSeparator = "─"
)

// ── Spinner ──

// Spinner shows an animated progress indicator.
type Spinner struct {
	mu      sync.Mutex
	active  bool
	message string
	done    chan struct{}
	frames  []string
}

// NewSpinner creates a new spinner.
func NewSpinner() *Spinner {
	return &Spinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		done:   make(chan struct{}),
	}
}

// Start begins the spinner animation with a message.
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		s.Update(message)
		return
	}
	s.active = true
	s.message = message
	s.done = make(chan struct{})
	s.mu.Unlock()

	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				s.mu.Lock()
				msg := s.message
				s.mu.Unlock()

				frame := s.frames[i%len(s.frames)]
				fmt.Printf("\r%s%s %s%s%s", ClearLine, ColorAccent, frame, Reset, " "+msg)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

// Update changes the spinner message without stopping.
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// Stop stops the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	close(s.done)
	fmt.Printf("\r%s", ClearLine)
}

// StopWith stops the spinner and shows a final message.
func (s *Spinner) StopWith(icon, color, message string) {
	s.mu.Lock()
	wasActive := s.active
	if wasActive {
		s.active = false
		close(s.done)
	}
	s.mu.Unlock()

	// Small delay to let the spinner goroutine exit.
	if wasActive {
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("\r%s  %s%s %s%s\n", ClearLine, color, icon, message, Reset)
}

// ── Formatted Output ──

// Banner prints the startup banner.
func Banner(version, model string, agentCount, skillCount int) {
	fmt.Println()
	fmt.Printf("  %s%s╭──────────────────────────────────────╮%s\n", Bold, ColorAccent, Reset)
	fmt.Printf("  %s%s│%s  %s%s GOAgent %s%s                        %s%s│%s\n", Bold, ColorAccent, Reset, Bold, BrightWhite, version, Reset, Bold, ColorAccent, Reset)
	fmt.Printf("  %s%s│%s  %sSovereign AI Agent Framework%s         %s%s│%s\n", Bold, ColorAccent, Reset, ColorDim, Reset, Bold, ColorAccent, Reset)
	fmt.Printf("  %s%s╰──────────────────────────────────────╯%s\n", Bold, ColorAccent, Reset)
	fmt.Println()
	fmt.Printf("  %s%s Model:%s  %s\n", ColorDim, IconInfo, Reset, model)
	fmt.Printf("  %s%s Agents:%s %d ready\n", ColorDim, IconInfo, Reset, agentCount)
	if skillCount > 0 {
		fmt.Printf("  %s%s Skills:%s %d loaded\n", ColorDim, IconInfo, Reset, skillCount)
	}
	fmt.Println()
	Separator()
	fmt.Printf("  %sType a message to begin. 'exit' to quit.%s\n", ColorDim, Reset)
	fmt.Println()
}

// Separator prints a horizontal line.
func Separator() {
	line := strings.Repeat(IconSeparator, 42)
	fmt.Printf("  %s%s%s\n", ColorDim, line, Reset)
}

// Prompt prints the input prompt.
func Prompt() {
	fmt.Printf("\n  %s%s%s %s", Bold, ColorPrompt, IconPrompt, Reset)
}

// Response prints a formatted agent response.
func Response(text string) {
	fmt.Printf("\r%s", ClearLine) // Clear any spinner residue.
	fmt.Println()

	// Print each line with a subtle left border.
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		fmt.Printf("  %s│%s %s%s%s\n", ColorAccent, Reset, ColorResponse, line, Reset)
	}
	fmt.Println()
}

// AgentStatus prints a colored agent delegation message.
func AgentStatus(agentName, task string) {
	fmt.Printf("\r%s  %s%s %s%s%s %s%s%s\n",
		ClearLine,
		ColorAgent, IconDelegate,
		Bold, agentName, Reset,
		ColorDim, truncateStr(task, 60), Reset)
}

// Success prints a success message.
func Success(message string) {
	fmt.Printf("  %s%s%s %s%s\n", ColorSuccess, Bold, IconSuccess, message, Reset)
}

// Error prints an error message.
func Error(message string) {
	fmt.Printf("  %s%s%s %s%s\n", ColorError, Bold, IconError, message, Reset)
}

// Warning prints a warning message.
func Warning(message string) {
	fmt.Printf("  %s%s%s %s%s\n", ColorWarning, Bold, IconWarning, message, Reset)
}

// Info prints an info message.
func Info(message string) {
	fmt.Printf("  %s%s %s%s\n", ColorInfo, IconInfo, message, Reset)
}

// AgentReady prints a formatted agent-ready line for startup.
func AgentReady(name, role string, toolCount int) {
	fmt.Printf("  %s%s%s %s%s%s%s — %s (%d tools)%s\n",
		ColorSuccess, Bold, IconSuccess,
		ColorHighlight, name, Reset,
		ColorDim, role, toolCount, Reset)
}

// Goodbye prints the shutdown message.
func Goodbye() {
	fmt.Println()
	Separator()
	fmt.Printf("  %s%s Session archived. Goodbye.%s\n", ColorDim, IconInfo, Reset)
	fmt.Println()
}

// truncateStr trims a string to max length with ellipsis.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

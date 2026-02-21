package shell

import (
	"context"
	"io"
	"strings"
	"time"
)

const MaxHistorySize = 1000

func (s *Shell) getHistoryFilePath() string {
	home := s.Env.Get("HOME")
	if home == "" {
		home = "/"
	}
	return home + "/.bash_history"
}

func (s *Shell) loadHistory() {
	ctx := context.Background()
	histFile := s.getHistoryFilePath()

	rc, err := s.vos.Open(ctx, histFile)
	if err != nil {
		return
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > MaxHistorySize {
		start = len(lines) - MaxHistorySize
	}
	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line != "" {
			s.history = append(s.history, line)
		}
	}
	s.savedOffset = len(s.history)
}

func (s *Shell) saveHistory() {
	if len(s.history) <= s.savedOffset {
		return
	}
	ctx := context.Background()
	histFile := s.getHistoryFilePath()
	newCommands := s.history[s.savedOffset:]
	content := strings.Join(newCommands, "\n") + "\n"

	existing := ""
	rc, err := s.vos.Open(ctx, histFile)
	if err == nil {
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		existing = string(data)
	}

	if err := s.vos.Write(ctx, histFile, strings.NewReader(existing+content)); err != nil {
		return
	}
	s.savedOffset = len(s.history)
}

// ExtractCommand extracts the command part from a history entry.
func ExtractCommand(entry string) string {
	if idx := strings.Index(entry, " ## "); idx != -1 {
		return entry[:idx]
	}
	return entry
}

func (s *Shell) addToHistory(cmd string) {
	if strings.TrimSpace(cmd) == "" {
		return
	}
	if len(s.history) > 0 {
		lastCmd := ExtractCommand(s.history[len(s.history)-1])
		if lastCmd == cmd {
			return
		}
	}
	timestamp := time.Now().Format(time.RFC3339)
	entry := cmd + " ## " + timestamp
	s.history = append(s.history, entry)
	s.saveHistory()
}

// History returns a copy of the command history.
func (s *Shell) History() []string {
	cp := make([]string, len(s.history))
	copy(cp, s.history)
	return cp
}

// ClearHistory clears the command history.
func (s *Shell) ClearHistory() { s.history = nil }

// HistorySize returns the number of commands in history.
func (s *Shell) HistorySize() int { return len(s.history) }

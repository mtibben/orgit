package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/99designs/grit/syncprinter"
	"github.com/egymgmbh/go-prefix-writer/prefixer"
	"github.com/fatih/color"
)

const ansiSaveCursorPosition = "\033[s"
const ansiClearLine = "\033[u\033[K"

type ProgressLogger struct {
	// stats

	total    atomic.Uint32
	complete atomic.Uint32
	errors   atomic.Uint32

	// state

	progressLineRunning bool

	// config

	Printer             *syncprinter.Printer
	WriterFor           func(localDir string) io.Writer
	LogSyncedRepo       bool
	LogExecCmd          bool
	LogRealtimeProgress bool
	LogInfo             bool
}

func NewProgressLogger(logLevel string) *ProgressLogger {
	switch logLevel {
	case "debug":
		return &ProgressLogger{
			LogExecCmd: true,
			WriterFor: func(localDir string) io.Writer {
				return prefixer.New(os.Stderr, func() string {
					return prefix(localDir)
				})
			},
			LogInfo: true,
		}
	case "verbose":
		return &ProgressLogger{
			Printer:             syncprinter.NewPrinter(os.Stderr),
			WriterFor:           func(localDir string) io.Writer { return io.Discard },
			LogSyncedRepo:       true,
			LogRealtimeProgress: true,
			LogInfo:             true,
		}

	case "quiet":
		return &ProgressLogger{
			Printer:   syncprinter.NewPrinter(os.Stderr),
			WriterFor: func(localDir string) io.Writer { return io.Discard },
		}
	default:
		return &ProgressLogger{
			Printer:             syncprinter.NewPrinter(os.Stderr),
			WriterFor:           func(localDir string) io.Writer { return io.Discard },
			LogRealtimeProgress: true,
			LogInfo:             true,
		}
	}
}

func prefix(localDir string) string {
	relDir, _ := filepath.Rel(getWorkspaceDir(), localDir)
	return color.HiBlackString("%s ", relDir)
}

func (p *ProgressLogger) EventExecCmd(cmd, dir string) {
	if p.LogExecCmd {
		w := p.WriterFor(dir)
		fmt.Fprintln(w, color.CyanString("+ %s", cmd))
	}
}

func (p *ProgressLogger) AddTotalToProgress(n uint32) {
	p.total.Add(n)
	p.PrintProgressLine()
}

func (p *ProgressLogger) EventSyncedRepoError(localDir string) {
	p.errors.Add(1)
}

func (p *ProgressLogger) EventSyncedRepo(localDir string) {
	p.complete.Add(1)
	if p.LogSyncedRepo {
		p.Printer.Printf("%sSynced %s\n%s", ansiClearLine, localDir, ansiSaveCursorPosition)
	}
	p.PrintProgressLine()
}

func (p *ProgressLogger) Info(s string) {
	if p.LogInfo {
		prefix := ""
		if p.LogRealtimeProgress && p.progressLineRunning {
			prefix = "\n"
		}
		p.Printer.Printf("%s%s\n", prefix, s)
		p.progressLineRunning = false
	}
}

func (p *ProgressLogger) PrintProgressLine() {
	if p.LogRealtimeProgress {
		total := p.total.Load()
		if total > 0 {
			firstChar := ansiClearLine
			if !p.progressLineRunning {
				firstChar = ansiSaveCursorPosition
			}
			p.progressLineRunning = true
			p.Printer.Printf("%sSyncing repos... %d/%d%s", firstChar, p.complete.Load(), total, numErrorsStr(p.errors.Load()))
		}
	}
}

func numErrorsStr(numErrors uint32) string {
	if numErrors == 1 {
		return " (1 error)"
	} else if numErrors > 1 {
		return fmt.Sprintf(" (%d errors)", numErrors)
	}
	return ""
}

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

type ProgressWriter interface {
	EventSyncedRepo(localdir string)
	EventAddTotal(n uint32)
	EventExecCmd(cmd, dir string)
	EventDone()
	WriterFor(localDir string) io.Writer
}

type PlainProgressWriter struct {
}

func (p *PlainProgressWriter) EventSyncedRepo(string) {}
func (p *PlainProgressWriter) EventAddTotal(n uint32) {}
func (p *PlainProgressWriter) EventDone()             {}
func (p *PlainProgressWriter) Println(s string)       {}

func (p *PlainProgressWriter) prefix(localDir string) string {
	relDir, _ := filepath.Rel(getWorkspaceDir(), localDir)
	return color.HiBlackString("%s ", relDir)
}

func (p *PlainProgressWriter) EventExecCmd(cmd, dir string) {
	w := p.WriterFor(dir)
	fmt.Fprintln(w, color.CyanString("+ %s", cmd))
}

func (p *PlainProgressWriter) WriterFor(localDir string) io.Writer {
	return prefixer.New(os.Stderr, func() string {
		return p.prefix(localDir)
	})
}

func NewProgressWriter(progresstype string) ProgressWriter {
	if progresstype == "plain" {
		return &PlainProgressWriter{}
	} else {
		return &MultiProgressWriter{
			printer: syncprinter.NewPrinter(os.Stderr),
		}
	}
}

type MultiProgressWriter struct {
	complete atomic.Uint32
	total    atomic.Uint32
	printer  *syncprinter.Printer
	printed  bool
}

func (p *MultiProgressWriter) EventAddTotal(n uint32) {
	p.total.Add(n)
	p.PrintProgress()
}

func (p *MultiProgressWriter) EventSyncedRepo(localDir string) {
	p.complete.Add(1)
	p.printer.Print(ansiClearLine + "Synced " + localDir + "\n" + ansiSaveCursorPosition)
	p.PrintProgress()
}

func (p *MultiProgressWriter) EventDone() {
	p.printer.Println("\nDone")
}

func (p *MultiProgressWriter) EventExecCmd(cmd, dir string) {}

func (p *MultiProgressWriter) PrintProgress() {
	total := p.total.Load()
	if total > 0 {
		firstChar := ansiClearLine
		if !p.printed {
			firstChar = ansiSaveCursorPosition
			p.printed = true
		}
		p.printer.Printf("%sSyncing repos... %d/%d", firstChar, p.complete.Load(), total)
	}
}

func (p *MultiProgressWriter) WriterFor(_ string) io.Writer {
	return io.Discard
}

package cmd

import (
	"os"
	"sync/atomic"

	"github.com/99designs/grit/syncprinter"
)

const ansiSaveCursorPosition = "\033[s"
const ansiClearLine = "\033[u\033[K"

type ProgressWriter struct {
	enabled  bool
	complete atomic.Uint32
	total    atomic.Uint32
	printer  *syncprinter.Printer
	printed  bool
}

func (p *ProgressWriter) AddTotal(n uint32) {
	p.total.Add(n)
	p.PrintProgress()
}

func (p *ProgressWriter) AddComplete(n uint32) {
	p.complete.Add(n)
	p.PrintProgress()
}

func NewProgressWriter(enabled bool) *ProgressWriter {
	return &ProgressWriter{
		printer: syncprinter.NewPrinter(os.Stderr),
		enabled: enabled,
	}
}

func (p *ProgressWriter) PrintProgress() {
	if !p.enabled {
		return
	}

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

func (p *ProgressWriter) Println(s string) {
	if !p.enabled {
		return
	}

	p.printer.Print(ansiClearLine + s + "\n" + ansiSaveCursorPosition)
	p.PrintProgress()
}

func (p *ProgressWriter) Done() {
	if !p.enabled {
		return
	}

	p.printer.Println("\nDone")
}

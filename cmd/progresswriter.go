package cmd

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const ansiSaveCursorPosition = "\033[s"
const ansiClearLine = "\033[u\033[K"

type ProgressWriter struct {
	Complete atomic.Uint32
	Total    atomic.Uint32

	doneChan  chan bool
	waitGroup sync.WaitGroup
	ticker    *time.Ticker
}

func NewProgressWriter() *ProgressWriter {
	return &ProgressWriter{
		doneChan:  make(chan bool),
		waitGroup: sync.WaitGroup{},
		ticker:    time.NewTicker(500 * time.Millisecond),
	}
}

func (p *ProgressWriter) PrintProgress(newline bool) {
	total := p.Total.Load()
	if total > 0 {
		fmt.Fprint(os.Stderr, ansiClearLine)
		fmt.Fprintf(os.Stderr, "Syncing repos... %d/%d", p.Complete.Load(), total)
		if newline {
			fmt.Fprint(os.Stderr, "\n")
		}
	}
}

func (p *ProgressWriter) Start() {
	p.waitGroup.Add(1)
	go func() {
		defer p.waitGroup.Done()

		fmt.Fprint(os.Stderr, ansiSaveCursorPosition)

		for {
			select {
			case <-p.ticker.C:
				p.PrintProgress(false)

			case <-p.doneChan:
				p.PrintProgress(true)
				return
			}
		}
	}()
}

func (p *ProgressWriter) Stop() {
	p.ticker.Stop()
	p.doneChan <- true
	p.waitGroup.Wait()
}

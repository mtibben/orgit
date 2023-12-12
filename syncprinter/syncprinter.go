package syncprinter

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var defaultPrinter = NewPrinter(os.Stderr)

type Printer struct {
	w           io.Writer
	outputMutex sync.Mutex
}

func NewPrinter(w io.Writer) *Printer {
	if w == nil {
		w = os.Stderr
	}
	return &Printer{
		w: w,
	}
}

func (p *Printer) do(f func()) {
	p.outputMutex.Lock()
	defer p.outputMutex.Unlock()
	f()
}

func (p *Printer) Println(a ...any) {
	p.do(func() {
		fmt.Fprintln(p.w, a...)
	})
}

func (p *Printer) Print(a ...any) {
	p.do(func() {
		fmt.Fprint(p.w, a...)
	})
}

func (p *Printer) Printf(format string, a ...any) {
	p.do(func() {
		fmt.Fprintf(p.w, format, a...)
	})
}

func Println(a ...any) {
	defaultPrinter.Println(a...)
}

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
	return &Printer{
		w: w,
	}
}

func (p *Printer) Println(a ...any) {
	p.outputMutex.Lock()
	defer p.outputMutex.Unlock()
	fmt.Fprintln(p.w, a...)
}

func (p *Printer) Print(a ...any) {
	p.outputMutex.Lock()
	defer p.outputMutex.Unlock()
	fmt.Fprint(p.w, a...)
}

func (p *Printer) Printf(format string, a ...any) {
	p.outputMutex.Lock()
	defer p.outputMutex.Unlock()
	fmt.Fprintf(p.w, format, a...)
}

func Println(a ...any) {
	defaultPrinter.Println(a...)
}

package report

import (
	"fmt"
	"io"
	"sync"
)

type VerboseLogFunc func(format string, args ...interface{})

type Entry struct {
	Method   string
	Path     string
	Handler  string
	Skipped  bool
	Reason   string
	Warnings []string
}

type Report struct {
	mu      sync.Mutex
	Entries []Entry
	Verbose bool
	Out     io.Writer
	VerboseOut io.Writer
}

func New(verbose bool, out, verboseOut io.Writer) *Report {
	return &Report{
		Verbose:    verbose,
		Out:        out,
		VerboseOut: verboseOut,
	}
}

func (r *Report) AddGenerated(method, path, handler string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Entries = append(r.Entries, Entry{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

func (r *Report) AddSkipped(method, path, handler, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Entries = append(r.Entries, Entry{
		Method:  method,
		Path:    path,
		Handler: handler,
		Skipped: true,
		Reason:  reason,
	})
}

func (r *Report) AddWarning(handler, warning string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.Entries {
		if r.Entries[i].Handler == handler {
			r.Entries[i].Warnings = append(r.Entries[i].Warnings, warning)
			return
		}
	}
}

func (r *Report) VerboseLog(format string, args ...interface{}) {
	if r.Verbose && r.VerboseOut != nil {
		fmt.Fprintf(r.VerboseOut, format+"\n", args...)
	}
}

func (r *Report) PrintDryRun() {
	generated := []Entry{}
	skipped := []Entry{}

	for _, e := range r.Entries {
		if e.Skipped {
			skipped = append(skipped, e)
		} else {
			generated = append(generated, e)
		}
	}

	if len(generated) > 0 {
		fmt.Fprintln(r.Out, "would generate swagger for:")
		for _, e := range generated {
			fmt.Fprintf(r.Out, "%s %s -> %s\n", e.Method, e.Path, e.Handler)
		}
	}

	if len(skipped) > 0 {
		if len(generated) > 0 {
			fmt.Fprintln(r.Out)
		}
		fmt.Fprintln(r.Out, "skipped:")
		for _, e := range skipped {
			fmt.Fprintf(r.Out, "%s %s -> %s (%s)\n", e.Method, e.Path, e.Handler, e.Reason)
		}
	}
}

func (r *Report) PrintWarnings() {
	for _, e := range r.Entries {
		for _, w := range e.Warnings {
			fmt.Fprintf(r.Out, "warning: %s: %s\n", e.Handler, w)
		}
	}
}

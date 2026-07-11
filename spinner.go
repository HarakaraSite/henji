package main

import (
	"fmt"
	"io"
	"time"
)

const spinnerLabel = "Generating"

// progressSpinner is a small stderr-only progress indicator. It deliberately
// owns no input or terminal state beyond one rewritten line, so the CLI stays
// non-interactive and does not need a UI framework.
type progressSpinner struct {
	done    chan struct{}
	stopped chan struct{}
}

func shouldShowSpinner(outputTTY, quiet bool) bool {
	return outputTTY && !quiet
}

func startSpinner(w io.Writer, quiet bool) *progressSpinner {
	if !shouldShowSpinner(isOutputTTY(), quiet) {
		return nil
	}
	s := &progressSpinner{done: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(s.stopped)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := 0
		render := func() {
			_, _ = fmt.Fprintf(w, "\r%s %s", frames[frame], spinnerLabel)
			frame = (frame + 1) % len(frames)
		}
		render()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				render()
			case <-s.done:
				_, _ = fmt.Fprint(w, "\r\033[2K")
				return
			}
		}
	}()
	return s
}

func (s *progressSpinner) stop() {
	if s == nil {
		return
	}
	close(s.done)
	<-s.stopped
}

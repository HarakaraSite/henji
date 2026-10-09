package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// renderMarkdownWithGlow is called only for completed text on a TTY. Keeping
// the renderer outside this process avoids its package initialization on every
// invocation, including decisions and machine-readable output.
func renderMarkdownWithGlow(ctx context.Context, text string, width int) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath("glow")
	if err != nil {
		return nil, err
	}
	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" {
		style = "auto"
	}
	if width < 0 {
		width = 0 // Both zero and negative widths previously disabled wrapping.
	}
	// Explicit style selection prevents Glow from choosing its plain non-TTY
	// style while we capture output. Buffering lets a failed renderer fall back
	// to the original text without printing a partial rendering first.
	cmd := exec.CommandContext(ctx, path, "-w", strconv.Itoa(width), "-s", style, "-")
	cmd.Stdin = strings.NewReader(text)
	// Environment settings override Glow's config without passing boolean
	// flags: some versions enable the pager whenever its flag is present, even
	// when passed as --pager=false. This is rendering, not an interactive viewer.
	cmd.Env = append(os.Environ(), "GLOW_PAGER=false", "GLOW_TUI=false")
	return cmd.Output()
}

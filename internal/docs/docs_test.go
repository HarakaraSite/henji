package docs

import (
	"strings"
	"testing"
)

const maxManualBodyBytes = 12 * 1024

func TestManual(t *testing.T) {
	manual := Manual("v2.0.0-test")
	if !strings.HasPrefix(manual, "# henji v2.0.0-test — manual\n") {
		t.Fatalf("manual has unexpected heading: %q", manual[:min(len(manual), 80)])
	}
	if !strings.Contains(manual, "## MCP tools") {
		t.Fatal("manual is missing the MCP section")
	}
	if strings.Contains(manual, "\x1b[") {
		t.Fatal("manual must not contain ANSI escapes")
	}
}

func TestManualSizeBudget(t *testing.T) {
	if size := len(Body()); size > maxManualBodyBytes {
		t.Fatalf("manual body is %d bytes; split topics before exceeding %d", size, maxManualBodyBytes)
	}
}

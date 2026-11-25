package main

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"

	"github.com/charledeon77/gostack-framework/framework/http"
)

var renderLogCaptureMu sync.Mutex

// TestShippedComponents_RenderWithEmptyData compiles/registers all generated components
// and executes them through http.Tempose.Render with empty data (D49).
// Optional-only components must render cleanly with zero [Tempose Warning] logs;
// modal must fail loudly when required `id` is missing and render cleanly when `id` is provided.
func TestShippedComponents_RenderWithEmptyData(t *testing.T) {
	renderLogCaptureMu.Lock()
	defer renderLogCaptureMu.Unlock()

	var logBuf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(origOut)

	tempose := http.NewTempose()
	RegisterComponents(tempose)

	optionalComponents := []string{"accordion", "button", "login", "register"}
	for _, name := range optionalComponents {
		var out bytes.Buffer
		if err := tempose.Render(&out, name, map[string]any{}, nil); err != nil {
			t.Errorf("component %s failed to render with empty data: %v", name, err)
		}
		if out.Len() == 0 {
			t.Errorf("component %s rendered empty HTML", name)
		}
	}

	// modal requires `id`: rendering with empty data must return an error (no shared 'modal' id collision)
	var modalErrOut bytes.Buffer
	if err := tempose.Render(&modalErrOut, "modal", map[string]any{}, nil); err == nil {
		t.Errorf("expected modal component without required 'id' to fail render, but it succeeded")
	}

	// modal with `id` provided must render cleanly with zero warnings
	var modalOKOut bytes.Buffer
	if err := tempose.Render(&modalOKOut, "modal", map[string]any{"id": "confirm-delete"}, nil); err != nil {
		t.Errorf("expected modal with id to render cleanly, got error: %v", err)
	}
	if !strings.Contains(modalOKOut.String(), `id="gs-modal-confirm-delete"`) {
		t.Errorf("expected modal output to contain id=\"gs-modal-confirm-delete\", got:\n%s", modalOKOut.String())
	}

	if strings.Contains(logBuf.String(), "[Tempose Warning]") {
		t.Fatalf("shipped components emitted Tempose reflection warning(s) during real render:\n%s", logBuf.String())
	}
}

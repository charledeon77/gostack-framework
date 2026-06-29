package foundation

import (
	"testing"
)

// traceProvider logs execution steps to assert strict order compliance.
type traceProvider struct {
	tracker *[]string
	name    string
}

func (tp *traceProvider) Register(c *Container) {
	*tp.tracker = append(*tp.tracker, tp.name+"_register")
}

func (tp *traceProvider) Boot(c *Container) {
	*tp.tracker = append(*tp.tracker, tp.name+"_boot")
}

// TestApplicationLifecycleExecutionOrdering verifies that the master application core
// runs ALL registrations across all modules before executing a single bootstrap boot routine.
func TestApplicationLifecycleExecutionOrdering(t *testing.T) {
	app := NewApplication()
	var traceLog []string

	providerA := &traceProvider{tracker: &traceLog, name: "ModuleA"}
	providerB := &traceProvider{tracker: &traceLog, name: "ModuleB"}

	// Register our two mock test drivers into the manager stack
	app.RegisterProvider(providerA)
	app.RegisterProvider(providerB)

	// Execute the master application boot loop sequence
	app.Boot()

	// The expected array blueprint tracking order. 
	// Both register methods MUST execute completely before either boot method runs.
	expectedSequence := []string{
		"ModuleA_register",
		"ModuleB_register",
		"ModuleA_boot",
		"ModuleB_boot",
	}

	if len(traceLog) != len(expectedSequence) {
		t.Fatalf("Sequence fault: Expected %d execution steps, but recorded %d actions", len(expectedSequence), len(traceLog))
	}

	for i, step := range traceLog {
		if step != expectedSequence[i] {
			t.Errorf("Order violation at sequence index %d: Expected step [%s], but recorded [%s]", i, expectedSequence[i], step)
		}
	}
}

// TestDynamicHotSwapProviderRegistration ensures that registering a new provider *after* // the master application has already booted runs its cycles inline immediately.
func TestDynamicHotSwapProviderRegistration(t *testing.T) {
	app := NewApplication()
	var traceLog []string

	app.Boot() // Initialize framework state into booted mode early

	hotSwapModule := &traceProvider{tracker: &traceLog, name: "HotSwap"}
	app.RegisterProvider(hotSwapModule)

	if len(traceLog) != 2 {
		t.Fatalf("Hot-swap failed: Expected inline execution of both Register and Boot steps.")
	}

	if traceLog[0] != "HotSwap_register" || traceLog[1] != "HotSwap_boot" {
		t.Errorf("Hot-swap execution error: Sequences recorded incorrectly: %v", traceLog)
	}
}
package foundation

import (
	"github.com/charledeon77/gostack-framework/framework/contract"
	"sync"
	"testing"
)

// Concrete structural representations used to verify container type alignment behaviors.
type mockDatabase struct {
	DriverName string
}

func (m *mockDatabase) Connect() error {
	return nil
}

func (m *mockDatabase) Query(sql string, args ...any) (interface{}, error) {
	return nil, nil
}

func (m *mockDatabase) Exec(sql string, args ...any) error {
	return nil
}

func (m *mockDatabase) BeginTx() (contract.Tx, error) {
	return nil, nil
}

func (m *mockDatabase) Driver() string {
	return m.DriverName
}

type mockLogger struct {
	Prefix string
}

// mockController demonstrates a nested dependency consumer requiring a logger to operate.
type mockController struct {
	Logger *mockLogger
}

// TestContainerSingletonLifecycle ensures that a Singleton factory function runs exactly once,
// caching the output and serving the identical pointer allocation across multiple resolutions.
func TestContainerSingletonLifecycle(t *testing.T) {
	container := NewContainer()
	executionCounter := 0

	container.BindSingleton("db", func(c *Container) any {
		executionCounter++
		return &mockDatabase{DriverName: "postgres"}
	})

	if executionCounter != 0 {
		t.Fatalf("Violation: Singleton factory triggered during binding phase instead of deferring to resolution.")
	}

	// First Resolution Pass (Triggers materialization)
	res1, err := container.Resolve("db")
	if err != nil {
		t.Fatalf("Failed to resolve singleton service blueprint: %v", err)
	}

	// Second Resolution Pass (Must hit memory cache directly)
	res2, err := container.Resolve("db")
	if err != nil {
		t.Fatalf("Failed to resolve cached singleton service instance: %v", err)
	}

	if executionCounter != 1 {
		t.Errorf("Lifecycle error: Expected factory to run exactly 1 time, but it fired %d times.", executionCounter)
	}

	db1 := res1.(*mockDatabase)
	db2 := res2.(*mockDatabase)

	if db1 != db2 {
		t.Errorf("Memory violation: Singleton failed to return the exact same instance pointer allocation.")
	}
}

// TestContainerTransientLifecycle ensures that a transient binding completely bypasses the cache,
// returning a clean, brand-new memory reference on every invocation pass.
func TestContainerTransientLifecycle(t *testing.T) {
	container := NewContainer()

	container.BindTransient("logger", func(c *Container) any {
		return &mockLogger{Prefix: "GOSTACK_APP"}
	})

	res1, _ := container.Resolve("logger")
	res2, _ := container.Resolve("logger")

	log1 := res1.(*mockLogger)
	log2 := res2.(*mockLogger)

	if log1 == log2 {
		t.Errorf("Lifecycle violation: Transient service returned identical pointer allocations across requests.")
	}
}

// TestContainerNestedDependencyChaining verifies that a factory recipe can dynamically
// call container.Resolve() internally to compound dependencies seamlessly.
func TestContainerNestedDependencyChaining(t *testing.T) {
	container := NewContainer()

	// Register the foundational leaf service
	container.BindSingleton("logger", func(c *Container) any {
		return &mockLogger{Prefix: "CHAIN_LOG"}
	})

	// Register the consumer service which chains the dependency resolution pass internally
	container.BindTransient("controller", func(c *Container) any {
		rawLogger, _ := c.Resolve("logger")
		return &mockController{Logger: rawLogger.(*mockLogger)}
	})

	res, err := container.Resolve("controller")
	if err != nil {
		t.Fatalf("Nested dependency resolution pass crashed: %v", err)
	}

	ctrl := res.(*mockController)
	if ctrl.Logger == nil || ctrl.Logger.Prefix != "CHAIN_LOG" {
		t.Errorf("Data alignment failure: Nested container dependencies failed to resolve or link correctly.")
	}
}

// TestContainerConcurrentConcurrencySafety runs a high-stress race condition simulator
// to guarantee our sync.RWMutex completely shields our internal maps from multi-threaded corruption.
func TestContainerConcurrentConcurrencySafety(t *testing.T) {
	container := NewContainer()
	var wg sync.WaitGroup

	container.BindSingleton("shared_resource", func(c *Container) any {
		return &mockDatabase{DriverName: "concurrent_mysql"}
	})

	concurrencyIntensity := 100
	wg.Add(concurrencyIntensity)

	for i := 0; i < concurrencyIntensity; i++ {
		go func() {
			defer wg.Done()
			_, err := container.Resolve("shared_resource")
			if err != nil {
				t.Errorf("Concurrent resolution thread failed: %v", err)
			}
			_ = container.Has("shared_resource")
		}()
	}

	wg.Wait()
}
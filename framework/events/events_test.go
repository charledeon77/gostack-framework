/*
Purpose:
This file implements unit tests for the GoStack event dispatcher.
It validates registration, synchronous processing, errors, and async concurrency.

Philosophy:
Events should execute correctly without race conditions or memory conflicts. We test
the dispatcher with multiple concurrent goroutines to assert thread-safety under HTTP load.
*/
package events

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type TestEvent struct {
	Message string
}

func TestDispatcher_Listen_And_Dispatch(t *testing.T) {
	dispatcher := NewDispatcher()
	var triggeredCount int

	dispatcher.Listen("test.event", func(event any) error {
		ev, ok := event.(TestEvent)
		if !ok {
			return errors.New("invalid event type")
		}
		if ev.Message == "hello" {
			triggeredCount++
		}
		return nil
	})

	// Run sync dispatch
	err := dispatcher.Dispatch("test.event", TestEvent{Message: "hello"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if triggeredCount != 1 {
		t.Errorf("expected event to trigger once, triggered %d times", triggeredCount)
	}
}

func TestDispatcher_Dispatch_Error_Stops_Chain(t *testing.T) {
	dispatcher := NewDispatcher()
	var secondTriggered bool

	dispatcher.Listen("test.event", func(event any) error {
		return errors.New("first handler error")
	})

	dispatcher.Listen("test.event", func(event any) error {
		secondTriggered = true
		return nil
	})

	err := dispatcher.Dispatch("test.event", TestEvent{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if secondTriggered {
		t.Error("expected subsequent handlers to be skipped after error")
	}
}

func TestDispatcher_DispatchAsync(t *testing.T) {
	dispatcher := NewDispatcher()
	var wg sync.WaitGroup
	wg.Add(1)

	var asyncRan bool
	dispatcher.Listen("async.event", func(event any) error {
		defer wg.Done()
		asyncRan = true
		return nil
	})

	dispatcher.DispatchAsync("async.event", TestEvent{}, nil)

	// Wait with a timeout to avoid hanging indefinitely if the test fails
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: async listener did not execute in time")
	}

	if !asyncRan {
		t.Error("expected async handler to have run")
	}
}

func TestDispatcher_Concurrent_Safety(t *testing.T) {
	dispatcher := NewDispatcher()

	// Register listener
	dispatcher.Listen("event", func(event any) error {
		return nil
	})

	// Run multiple parallel dispatches and registers
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			dispatcher.Listen("event", func(event any) error {
				return nil
			})
		}()
		go func() {
			defer wg.Done()
			_ = dispatcher.Dispatch("event", TestEvent{})
		}()
	}
	wg.Wait()
}

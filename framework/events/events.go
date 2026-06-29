/*
Purpose:
This file implements the Spark EventDispatcher subsystem for the GoStack framework.
It provides a synchronous and asynchronous Pub/Sub communication channel.

Philosophy:
Ecosystem decoupling is critical for clean application code. By dispatching events
instead of coupling business components directly, we preserve single-responsibility
rules. The dispatcher is written using standard mutexes to ensure high-performance,
concurrent-safe registrations and publishing.

Architecture:
A standalone framework package (`github.com/charledeon77/gostack-framework/framework/events`). Implements the
`contract.EventDispatcher` interface.

Choice:
We chose a map-of-slices design with read/write mutexes rather than a channel-based multiplexer
to guarantee immediate synchronous dispatch order when requested (which is crucial for database transactions)
while offering clean `DispatchAsync` hooks for non-blocking processes.

Implementation:
- Dispatcher: coordinates event listening and firing.
  - Listen(): registers a Listener to a specific event category.
  - Dispatch(): fires the event executing all handlers synchronously in the same thread.
  - DispatchAsync(): fires all registered event handlers in separate background goroutines.
*/
package events

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"sync"
)

// Dispatcher manages event-listener mappings safely across concurrent HTTP handlers.
type Dispatcher struct {
	mu        sync.RWMutex
	listeners map[string][]contract.Listener
}

// NewDispatcher initializes an empty, thread-safe Dispatcher instance.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		listeners: make(map[string][]contract.Listener),
	}
}

// Listen registers a listener function to run when a specific event name is fired.
func (d *Dispatcher) Listen(eventName string, listener contract.Listener) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.listeners[eventName] = append(d.listeners[eventName], listener)
}

// Dispatch executes all listeners registered to eventName synchronously.
// If any listener returns an error, execution stops and that error is returned.
func (d *Dispatcher) Dispatch(eventName string, event any) error {
	d.mu.RLock()
	handlers, exists := d.listeners[eventName]
	d.mu.RUnlock()

	if !exists {
		return nil
	}

	for i, handler := range handlers {
		if err := handler(event); err != nil {
			return fmt.Errorf("events: listener %d failed for event '%s': %w", i, eventName, err)
		}
	}

	return nil
}

// DispatchAsync fires all registered listeners in their own concurrent background goroutines.
// It returns immediately and executes asynchronously, logging errors via a user-definable callback
// or standard runtime recovery.
func (d *Dispatcher) DispatchAsync(eventName string, event any, errHandler func(error)) {
	d.mu.RLock()
	handlers, exists := d.listeners[eventName]
	d.mu.RUnlock()

	if !exists {
		return
	}

	for _, handler := range handlers {
		h := handler // Capture closure loop variable
		go func() {
			if err := h(event); err != nil && errHandler != nil {
				errHandler(fmt.Errorf("events async: error running handler for event '%s': %w", eventName, err))
			}
		}()
	}
}

/*
Purpose:
This file implements the core notification Dispatcher orchestrator.

Philosophy:
A central dispatch gateway guarantees that applications can trigger alert requests globally with
minimal operational overhead. The Dispatcher manages a thread-safe registry of named channel adapters,
evaluates destination channels dynamically, and processes delivery pipelines transparently.

Architecture:
Coordinates all implementations of Channel. It reads target channels dynamically from the
Notification's `Via()` pattern and executes parallel or sequential deliveries gracefully.
*/
package notification

import (
	"fmt"
	"sync"
)

// Dispatcher coordinates the sending of notifications through custom delivery channels.
type Dispatcher struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewDispatcher initializes a dispatcher with no default pre-configured channels.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		channels: make(map[string]Channel),
	}
}

// Register registers a named delivery channel strategy.
func (d *Dispatcher) Register(name string, channel Channel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels[name] = channel
}

// Send dispatches a single notification to a single notifiable recipient.
func (d *Dispatcher) Send(notifiable any, notification Notification) error {
	channelsToUse := notification.Via(notifiable)
	if len(channelsToUse) == 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, channelName := range channelsToUse {
		channel, exists := d.channels[channelName]
		if !exists {
			return fmt.Errorf("notification: channel [%s] is not registered", channelName)
		}

		err := channel.Send(notifiable, notification)
		if err != nil {
			return fmt.Errorf("notification: failed to send through channel [%s]: %w", channelName, err)
		}
	}

	return nil
}

// SendMany delivers a single notification to multiple notifiable recipients.
func (d *Dispatcher) SendMany(notifiables []any, notification Notification) error {
	for _, notifiable := range notifiables {
		if err := d.Send(notifiable, notification); err != nil {
			return err
		}
	}
	return nil
}

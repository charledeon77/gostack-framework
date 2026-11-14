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

PAR-22: Every configured channel is always attempted. Per-channel failures are aggregated with
errors.Join and returned together so the caller knows exactly which channels failed — a failure on
the mail channel cannot prevent the in-app database notification from being written.
SendMany runs to completion for every recipient: a failing middle recipient does not stop delivery
to the rest. The channel set is copied under RLock and all channel.Send calls happen outside the
lock so that a concurrent Register call does not deadlock.
*/
package notification

import (
	"errors"
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

// Send dispatches a single notification to a single notifiable recipient (PAR-22).
// Every configured channel is attempted. Failures are aggregated; a failure on one channel
// does not prevent delivery through the remaining channels.
func (d *Dispatcher) Send(notifiable any, notification Notification) error {
	channelsToUse := notification.Via(notifiable)
	if len(channelsToUse) == 0 {
		return nil
	}

	// PAR-22: Copy the channel set under RLock, then release lock before sending.
	d.mu.RLock()
	resolved := make(map[string]Channel, len(channelsToUse))
	var missingChannels []string
	for _, name := range channelsToUse {
		ch, exists := d.channels[name]
		if !exists {
			missingChannels = append(missingChannels, name)
			continue
		}
		resolved[name] = ch
	}
	d.mu.RUnlock()

	// Aggregate all errors — including missing channels and send failures.
	var errs []error
	for _, name := range missingChannels {
		errs = append(errs, fmt.Errorf("notification: channel [%s] is not registered", name))
	}

	for _, name := range channelsToUse {
		ch, ok := resolved[name]
		if !ok {
			continue // already recorded as missing above
		}
		if err := ch.Send(notifiable, notification); err != nil {
			errs = append(errs, fmt.Errorf("notification: channel [%s] failed: %w", name, err))
		}
	}

	return errors.Join(errs...)
}

// SendMany delivers a single notification to multiple notifiable recipients (PAR-22).
// Delivery continues for every recipient even if some fail. All errors are aggregated.
func (d *Dispatcher) SendMany(notifiables []any, notification Notification) error {
	var errs []error
	for _, notifiable := range notifiables {
		if err := d.Send(notifiable, notification); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

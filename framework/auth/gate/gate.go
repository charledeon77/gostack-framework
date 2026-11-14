/*
Purpose:
Package gate provides policy-based authorization gates for GoStack (DX-03).
It allows defining callback-based abilities and authorizing user actions before executing business logic.
*/
package gate

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrUnauthorized = errors.New("gate: action unauthorized")
)

// Gate manages permission definitions and authorization decisions.
type Gate struct {
	mu        sync.RWMutex
	abilities map[string]func(user any, target any) bool
}

// New creates an initialized authorization Gate.
func New() *Gate {
	return &Gate{
		abilities: make(map[string]func(user any, target any) bool),
	}
}

// Define registers an authorization callback for a given ability name.
func (g *Gate) Define(ability string, callback func(user any, target any) bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.abilities[ability] = callback
}

// Allows checks whether the given user has permission to perform the ability on target.
func (g *Gate) Allows(ability string, user any, target any) bool {
	g.mu.RLock()
	cb, ok := g.abilities[ability]
	g.mu.RUnlock()

	if !ok {
		return false
	}
	return cb(user, target)
}

// Denies is the logical inverse of Allows.
func (g *Gate) Denies(ability string, user any, target any) bool {
	return !g.Allows(ability, user, target)
}

// Authorize verifies that the ability is allowed; if not, returns ErrUnauthorized.
func (g *Gate) Authorize(ability string, user any, target any) error {
	if g.Denies(ability, user, target) {
		return fmt.Errorf("%w for ability %q", ErrUnauthorized, ability)
	}
	return nil
}


package auth

import (
	"github.com/charledeon77/gostack-framework/framework/contract"
	"reflect"
	"strings"
)

// Policy defines a callback function to determine if a user has access to a specific resource.
type Policy func(user contract.Authenticatable, model any) bool

// Gate manages permissions and policies.
type Gate struct {
	abilities map[string]func(user contract.Authenticatable, args ...any) bool
	policies  map[string]Policy
}

// NewGate constructs a fresh Gate manager.
func NewGate() *Gate {
	return &Gate{
		abilities: make(map[string]func(user contract.Authenticatable, args ...any) bool),
		policies:  make(map[string]Policy),
	}
}

// Define registers a named ability callback.
func (g *Gate) Define(ability string, fn func(user contract.Authenticatable, args ...any) bool) {
	g.abilities[ability] = fn
}

// RegisterPolicy registers a policy for a specific resource type name (e.g. "post").
func (g *Gate) RegisterPolicy(resourceName string, policy Policy) {
	g.policies[strings.ToLower(resourceName)] = policy
}

// Allows checks if the given user is allowed to perform the ability.
func (g *Gate) Allows(user contract.Authenticatable, ability string, args ...any) bool {
	if fn, ok := g.abilities[ability]; ok {
		return fn(user, args...)
	}
	
	// Fall back to policy check if resources are provided
	if len(args) > 0 {
		resourceName := getResourceName(args[0])
		if policy, ok := g.policies[resourceName]; ok {
			return policy(user, args[0])
		}
	}
	return false
}

// Denies checks if the given user is denied from performing the ability.
func (g *Gate) Denies(user contract.Authenticatable, ability string, args ...any) bool {
	return !g.Allows(user, ability, args...)
}

func getResourceName(resource any) string {
	t := reflect.TypeOf(resource)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return strings.ToLower(t.Name())
}

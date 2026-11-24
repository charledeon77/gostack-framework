package safego

import (
	"log"
	"runtime/debug"
)

// Go launches fn in a new goroutine wrapped in a panic-recovery boundary.
// A panic inside fn is caught, logged with its full stack trace, and the goroutine
// exits cleanly — the server process is never terminated by an unhandled panic.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[GoStack] PANIC recovered in goroutine %q: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}

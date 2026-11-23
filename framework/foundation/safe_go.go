package foundation

import "github.com/charledeon77/gostack-framework/framework/foundation/safego"

// SafeGo launches fn in a new goroutine wrapped in a panic-recovery boundary.
func SafeGo(name string, fn func()) {
	safego.Go(name, fn)
}

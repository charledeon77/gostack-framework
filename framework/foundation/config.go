/*
Purpose:
This file implements environment configuration access helper functions for the GoStack framework.

Philosophy:
We believe application configuration should be decoupled from logic. By storing configs in
environment variables, we ensure portability across local dev, staging, and production runtimes.

Architecture:
Part of the foundation package, providing environment lookup utilities to bootstrapper files (like main.go).

Choice:
We consolidated the framework/config package directly into the foundation package, removing
package import cycles and folder fragmentation.

Implementation:
- Get: retrieves environment variables with support for default fallbacks.
*/
package foundation

import (
	"os"
)

// Get retrieves an environment variable or returns a default value.
//
// Parameters:
//   - key: The name of the environment variable.
//   - fallback: The value to return if the variable is not set.
func Get(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

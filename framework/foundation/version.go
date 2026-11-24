package foundation

import "runtime/debug"

// Version is the fallback semantic release tag of the GoStack framework.
const Version = "v1.0.27"

// GetVersion returns the real-time version from Go module build information
// (embedded automatically by `go build` or `go install`). If the module info is unavailable,
// it falls back cleanly to the Version constant.
func GetVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		for _, dep := range info.Deps {
			if dep.Path == "github.com/charledeon77/gostack-framework" {
				if dep.Version != "" && dep.Version != "(devel)" {
					return dep.Version
				}
			}
		}
	}
	return Version
}

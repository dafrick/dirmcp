//go:build race

package shell

// The race-instrumented test binary cannot play a confined program.
func init() { raceBuild = true }

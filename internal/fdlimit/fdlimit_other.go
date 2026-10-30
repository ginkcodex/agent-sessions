//go:build !unix

package fdlimit

// Inherit is a no-op: Windows has no inherited open-file limit.
func Inherit() error { return nil }

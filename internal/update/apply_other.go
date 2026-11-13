//go:build !darwin

package update

// ApplyMain runs the update helper when the program was started as one. No
// platform here has one yet, so it never does.
func ApplyMain(args []string) (code int, ok bool) { return 0, false }

package files

import "testing"

// setLimit lowers a package limit for one test, so cap and truncation tests
// need not build full-size trees: every entry is a file-system operation and,
// for repositories, a Git process, which is slow on Windows. Tests that use it
// must not run in parallel.
func setLimit(t testing.TB, limit *int, value int) {
	t.Helper()
	old := *limit
	*limit = value
	t.Cleanup(func() { *limit = old })
}

// testFileCap stands in for MaxFiles in cap tests: large enough to span several
// directories and frontier batches, small enough to build quickly.
const testFileCap = 256

func lowFileCap(t testing.TB) int {
	setLimit(t, &fileCap, testFileCap)
	return fileCap
}

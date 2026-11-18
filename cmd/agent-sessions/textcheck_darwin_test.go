//go:build darwin && desktop

package main

import "testing"

// init registered the defaults before this test runs, as it does before the
// app creates its first web view.
func TestWebTextCheckingDefaultsAreOff(t *testing.T) {
	for _, key := range webTextCheckingDefaults {
		on, set := webTextCheckingDefault(key)
		if !set {
			t.Errorf("%s is not registered; WebKit would follow the system setting", key)
		} else if on {
			t.Errorf("%s is on; want the registered NO (or a saved user choice is overriding it)", key)
		}
	}
}

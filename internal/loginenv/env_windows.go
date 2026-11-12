package loginenv

import "time"

// Shell is empty on Windows: there is no login shell to read.
func Shell() string { return "" }

func read(string, string, time.Duration) []string { return nil }

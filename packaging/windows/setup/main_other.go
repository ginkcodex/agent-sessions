//go:build !windows

package main

import "fmt"

func main() { fmt.Println("Agent Sessions Setup is only supported on Windows.") }

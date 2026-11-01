//go:build windows && !setupbundle

package main

// Ordinary builds/tests need no generated payload. Running this executable
// reports a clear packaging error before any setup operation.
var payloadMSI []byte

const (
	payloadSHA256  = ""
	payloadVersion = ""
)

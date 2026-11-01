//go:build windows && setupbundle

package main

import _ "embed"

// The packaging script supplies payload.msi and bundle_metadata.go in an
// isolated staging module. Metadata defines payloadSHA256 and payloadVersion.
//
//go:embed payload.msi
var payloadMSI []byte

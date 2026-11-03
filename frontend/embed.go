//go:build desktop

package frontend

import "embed"

// Dist holds the built frontend assets (the output of `npm run build` in
// frontend/). The embed pattern requires frontend/dist to exist at compile
// time; `make app` and the packaging targets run `npm run build` before
// `go build`, and `make dev` builds it once if it is missing.
//
//go:embed all:dist
var Dist embed.FS

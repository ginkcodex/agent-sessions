package managedtools

import (
	_ "embed"
	goversion "go/version"
	"runtime"
)

// The catalog is closed. Every source, version and integrity value is pinned
// here; a request can only name a tool. Hashes and sums were taken from the
// official release pages, proxy.golang.org and sum.golang.org on 2026-10-23.
// Like editors, the app downloads a known version only when the project and
// the user's environment have none (docs/plan/code-diagnostics-zed-model.md).

// Tools a client may propose. Node.js is installed only as an npm server's runtime.
var proposable = []string{"gopls", "ruff", "vtsls", "pyright", "rust-analyzer", "svelte-language-server"}

type artifact struct{ URL, SHA256 string }

// gopls is built with the host's Go, so the newest release that Go can build
// is chosen (GOTOOLCHAIN=local never downloads a newer toolchain).
type goplsRelease struct{ Version, MinGo, Sum, ModSum string }

var goplsReleases = []goplsRelease{
	{"v0.24.0", "go1.27.0", "h1:nPO/LffYa4sP62Ueuema5PPnZPGaZjGl+vZGvbTek4g=", "h1:abHXJMs6FnwH1z/R1GrYVo/TBfxyDbSFMCKakBQURg0="},
	{"v0.23.0", "go1.26.0", "h1:Dn6mf9WXu9iLnTftDDMb9wV0c6Se7PjzEMqP0LEe08Y=", "h1:Ijg67bAdTicg9IINxII7MV+dQpHEXM4646WpNWVYBP0="},
	{"v0.21.1", "go1.25.0", "h1:1/o9z5Brdero4jFm9Jr46Uwj8GU9lQdoSXHMlwRHb/w=", "h1:eyYJvtYY6UF9WbXilvCkD3fdYMJwG5cVCS4bdOQ2qIs="},
}

const (
	goplsModule = "golang.org/x/tools/gopls"
	goProxy     = "https://proxy.golang.org"
	goplsPolicy = "go-module-sumdb-v1"

	ruffVersion = "0.15.20"
	ruffPolicy  = "release-archive-sha256-v1"

	nodeVersion = "v24.22.1"
	nodePolicy  = "release-archive-sha256-v1"

	vtslsVersion      = "0.3.0"
	vtslsTypeScript   = "5.9.3"
	npmPolicy         = "npm-ci-lockfile-ignore-scripts-v1"
	vtslsMain         = "node_modules/@vtsls/language-server/bin/vtsls.js"
	npmRegistry       = "https://registry.npmjs.org/"
	vtslsLockPackages = 19

	pyrightVersion      = "1.1.414"
	pyrightMain         = "node_modules/pyright/langserver.index.js"
	pyrightLockPackages = 2

	svelteLSVersion      = "0.18.5"
	svelteLSMain         = "node_modules/svelte-language-server/bin/server.js"
	svelteLSSvelte       = "4.2.20" // the server's own fallback compiler (nested)
	svelteLSTypeScript   = "6.0.3"
	svelteLSLockPackages = 63

	// The Vue TypeScript plugin runs inside vtsls's TypeScript server; it has
	// no command of its own.
	vuePluginVersion      = "3.3.13"
	vuePluginMain         = "node_modules/@vue/typescript-plugin/index.js"
	vuePluginLockPackages = 21

	rustAnalyzerVersion = "2026-10-26"
	// The receipt policy also carries the pinned asset digest for the platform.
	rustAnalyzerPolicy = "release-file-gzip-sha256-v1"
	// Keep the decompressed executable within fileDigest's binary size limit.
	rustAnalyzerLimit = 256 << 20
)

var ruffArchives = map[string]artifact{
	"darwin/arm64": {"https://github.com/astral-sh/ruff/releases/download/0.15.20/ruff-aarch64-apple-darwin.tar.gz", "cb41c48690c113dc08470e64103ce65ca15249c6ce3495d3ac792b329a83e8c1"},
	"darwin/amd64": {"https://github.com/astral-sh/ruff/releases/download/0.15.20/ruff-x86_64-apple-darwin.tar.gz", "36b91219b3aae00464e2a4fa361766abd47d0402ac88fbe6da5de44285738386"},
	// musl builds are static, so they run on any Linux distribution.
	"linux/amd64": {"https://github.com/astral-sh/ruff/releases/download/0.15.20/ruff-x86_64-unknown-linux-musl.tar.gz", "0715560b2402024b529a4ffd6ea7d3814a090eb255a6216259fb192823be3ad8"},
	"linux/arm64": {"https://github.com/astral-sh/ruff/releases/download/0.15.20/ruff-aarch64-unknown-linux-musl.tar.gz", "2e06ebae862dce74eb7b084081621f2d4183e18e60cb074e7906f862869fa1b6"},
}

var nodeArchives = map[string]artifact{
	"darwin/arm64": {"https://nodejs.org/dist/v24.22.1/node-v24.22.1-darwin-arm64.tar.gz", "89c682593d76cd44233fa53b1563264e7de9cbb189cad169135c5b4fadfbc0a7"},
	"darwin/amd64": {"https://nodejs.org/dist/v24.22.1/node-v24.22.1-darwin-x64.tar.gz", "d132557e7ee2b85af08b0c8c34338e65c6430f2238d01f727728ef42dc83b035"},
	"linux/arm64":  {"https://nodejs.org/dist/v24.22.1/node-v24.22.1-linux-arm64.tar.gz", "33c37be234ba8bee041961047b19160f4d7d0f164e8e246f78bdd10feff148b3"},
	"linux/amd64":  {"https://nodejs.org/dist/v24.22.1/node-v24.22.1-linux-x64.tar.gz", "eb6728e77c576a35162c122c45ccadf224d89ff00abc8dae46cc2897c6002d7c"},
}

// Official rust-lang release assets, pinned 2026-10-27. Each is a single gzip
// member whose payload is the rust-analyzer executable (no archive members).
var rustAnalyzerArchives = map[string]artifact{
	"darwin/arm64": {"https://github.com/rust-lang/rust-analyzer/releases/download/2026-10-26/rust-analyzer-aarch64-apple-darwin.gz", "8fb90cb58f5910d8d8b24ac1a93cca49f053365cbf0eb3c682144ad0a0027dcf"},
	"darwin/amd64": {"https://github.com/rust-lang/rust-analyzer/releases/download/2026-10-26/rust-analyzer-x86_64-apple-darwin.gz", "b3e4904877bcf7013a4a342c449cff1bf0ea0b91d5a1a30ac8df3d32142519c0"},
	"linux/arm64":  {"https://github.com/rust-lang/rust-analyzer/releases/download/2026-10-26/rust-analyzer-aarch64-unknown-linux-gnu.gz", "1b225c4c9ae329ca96e8a75a5bbcb5562a7f1d25c57c8b55d3e02299a3dbe836"},
	"linux/amd64":  {"https://github.com/rust-lang/rust-analyzer/releases/download/2026-10-26/rust-analyzer-x86_64-unknown-linux-gnu.gz", "4c83773525f93b636ed79761389568b4d2a89c05fa7ecc440245032d49ad80ce"},
}

// Each npm server's dependency tree is pinned by an npm lockfile: npm ci checks every
// package against its sha512 integrity value and fails on any difference.
var (
	//go:embed vtsls/package.json
	vtslsPackageJSON []byte
	//go:embed vtsls/package-lock.json
	vtslsLockfile []byte
	//go:embed pyright/package.json
	pyrightPackageJSON []byte
	//go:embed pyright/package-lock.json
	pyrightLockfile []byte
	//go:embed svelte-language-server/package.json
	svelteLSPackageJSON []byte
	//go:embed svelte-language-server/package-lock.json
	svelteLSLockfile []byte
	//go:embed vue-typescript-plugin/package.json
	vuePluginPackageJSON []byte
	//go:embed vue-typescript-plugin/package-lock.json
	vuePluginLockfile []byte
)

func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

func installDir(name, version string) string {
	return name + "-" + version + "-" + runtime.GOOS + "-" + runtime.GOARCH
}

// goplsFor picks the newest catalog release the given Go can build.
func goplsFor(goVersion string) (goplsRelease, bool) {
	for _, r := range goplsReleases {
		if goversion.Compare(goVersion, r.MinGo) >= 0 {
			return r, true
		}
	}
	return goplsRelease{}, false
}

package codediagnostics

import (
	"fmt"
	"go/version"
	"path"
	"strings"
)

// goRequirement is a contained module's go directive, as its go.mod states it.
type goRequirement struct{ module, version string }

// goTooOld explains the modules whose go directive needs a newer Go than the
// reviewed toolchain at goroot. Checkers run Go with GOTOOLCHAIN=local, so Go
// refuses such a module rather than download the toolchain it asks for. It
// returns how many modules are affected; a toolchain that is not a release
// and invalid directives are never reported. purpose names what the newer Go
// enables.
func goTooOld(goroot string, reqs []goRequirement, purpose string) (string, int) {
	installed := reviewedGoVersion(goroot)
	if installed == "" {
		return "", 0
	}
	var newest goRequirement
	affected := 0
	for _, r := range reqs {
		need := "go" + r.version
		if !version.IsValid(need) || version.Compare(need, installed) <= 0 {
			continue
		}
		affected++
		if newest.version == "" || version.Compare(need, "go"+newest.version) > 0 {
			newest = r
		}
	}
	if affected == 0 {
		return "", 0
	}
	where := path.Join(newest.module, "go.mod")
	if affected > 1 {
		where += fmt.Sprintf(" and %d more modules", affected-1)
	}
	return fmt.Sprintf("This project needs Go %s (%s), but the Go found is %s (%s). Install Go %s or newer as the default go %s, then use Rediscover checkers in Code checks. No Go toolchain is downloaded automatically.",
		newest.version, where, strings.TrimPrefix(installed, "go"), goroot, newest.version, purpose), affected
}

// reviewedGoVersion is the release in the reviewed GOROOT's VERSION file
// (metadata only), such as "go1.26.8", or "" when it is not a release.
func reviewedGoVersion(goroot string) string {
	v := goVersionFileName(goroot)
	if !version.IsValid(v) {
		return ""
	}
	return v
}

package managedtools

import (
	"strings"
	"testing"
)

func TestOlderThan(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		{"1.2.3", "1.2.3", false},
		{"2.5.9", "2.6.0", true},
		{"1.0", "2.0.0", false},
		{"01.0.0", "2.0.0", false},
		{"", "1.0.0", false},
	} {
		if got := OlderThan(c.a, c.b); got != c.want {
			t.Errorf("OlderThan(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// An older copy is checked by its adapter package's own version.
func TestOtherVersionChecksTheAdapterPackage(t *testing.T) {
	for _, tool := range adapterTools {
		pkg := otherVersion(npmPackages[tool](), "0.0.1")
		if len(pkg.versions) != 1 || pkg.version != "0.0.1" || pkg.policy() != npmOmitOptionalPolicy {
			t.Fatal(tool, pkg.versions, pkg.version)
		}
		for name, v := range pkg.versions {
			if v != "0.0.1" || !strings.HasPrefix(pkg.main, "node_modules/"+name+"/") {
				t.Fatal(tool, name, v)
			}
		}
	}
}

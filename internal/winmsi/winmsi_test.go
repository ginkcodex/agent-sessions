package winmsi

import "testing"

func TestVersionsAndGUIDs(t *testing.T) {
	for _, bad := range []string{"", "1.2", "1.2.3.4", "256.0.0", "0.256.0", "0.0.65536", "-1.0.0", "1.2.x", "1.2.3-beta", "1. 2.3"} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	v, err := ParseVersion("255.255.65535")
	if err != nil || v != (Version{255, 255, 65535}) {
		t.Fatal(v, err)
	}
	if !ValidGUID(UpgradeCode) || ValidGUID("{wrong}") {
		t.Fatal("GUID validation failed")
	}
	a, _ := ParseVersion("1.6.0")
	b, _ := ParseVersion("1.5.9")
	if !a.NewerThan(b) || b.NewerThan(a) || a.NewerThan(a) {
		t.Fatal("version order is wrong")
	}
}

func TestForRelease(t *testing.T) {
	for in, want := range map[string]string{"1.6.0": "1.6.0", "v1.6.0": "1.6.0", "1.6.0-rc1": "1.6.0", "1.6.0+build5": "1.6.0", " 1.6.0 ": "1.6.0"} {
		if got := ForRelease(in); got != want {
			t.Errorf("ForRelease(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInfoCheckAcceptsOnlyOurPerUserPackage(t *testing.T) {
	good := Info{
		ProductCode: "{11111111-2222-3333-4444-555555555555}", UpgradeCode: "{d7786be1-bf16-4887-9c3b-345c5d87cd95}",
		ProductName: ProductName, ProductVersion: "1.6.0", PerUser: "1",
	}
	if err := good.Check("1.6.0"); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(i *Info){
		"another product family": func(i *Info) { i.UpgradeCode = "{00000000-0000-0000-0000-000000000000}" },
		"bad product code":       func(i *Info) { i.ProductCode = "nope" },
		"another name":           func(i *Info) { i.ProductName = "Other App" },
		"machine-wide":           func(i *Info) { i.AllUsers = "1" },
		"per-machine":            func(i *Info) { i.PerUser = "0" },
		"other version":          func(i *Info) { i.ProductVersion = "1.5.9" },
		"unparsable version":     func(i *Info) { i.ProductVersion = "1.6" },
	} {
		t.Run(name, func(t *testing.T) {
			i := good
			change(&i)
			if err := i.Check("1.6.0"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if err := good.Check("1.6"); err == nil {
		t.Fatal("accepted a mistyped expected version")
	}
}

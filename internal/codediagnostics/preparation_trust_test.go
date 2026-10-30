package codediagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparationRebindRequiresCurrentExactLease(t *testing.T) {
	for _, scenario := range []string{"success", "reapproval", "external-revoke", "old-bytes", "new-bytes"} {
		t.Run(scenario, func(t *testing.T) {
			root, old, fp := fixture(t)
			grants := store(t)
			trust := NewTrust(grants)
			defer trust.Close()
			approve(t, trust, root, old, fp, "persistent")
			// A new owner must record the durable grant as the lease's epoch.
			trust.Close()
			trust = NewTrust(grants)
			defer trust.Close()
			lease, err := trust.Preflight(root, old)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root.Project().Root, "prepared-tool")
			if err := os.WriteFile(path, []byte("new approved output"), 0600); err != nil {
				t.Fatal(err)
			}
			next := append(cloneTools(old), ToolInput{ProviderID: "prepared", Executable: path, Policy: "fixed-prepared-v1"})
			expected, err := Fingerprint(root, next)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "reapproval":
				approve(t, trust, root, old, fp, "persistent")
			case "external-revoke":
				peer := NewTrust(grants)
				defer peer.Close()
				if err := peer.Revoke(root); err != nil {
					t.Fatal(err)
				}
			case "old-bytes":
				if err := os.WriteFile(old[0].Executable, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "new-bytes":
				if err := os.WriteFile(path, []byte("replaced output"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			approval, err := trust.Rebind(root, lease, old, next, expected)
			if scenario == "success" {
				if err != nil || approval.State != "approved" || approval.Fingerprint != expected {
					t.Fatalf("carry over: %+v %v", approval, err)
				}
				if err := lease.Validate(root, old); err == nil {
					t.Fatal("old lease survived rebind")
				}
				if _, err := trust.Preflight(root, next); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("carried a stale or changed approval")
			}
			if scenario == "external-revoke" {
				found, err := hasGrant(grants, root.Project(), expected)
				if err != nil || found {
					t.Fatalf("restored externally revoked grant: %v %v", found, err)
				}
			}
		})
	}
}

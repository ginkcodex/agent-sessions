package codediagnostics

import (
	"errors"
	"testing"
)

func TestPreparationPublicationFailureDoesNotMoveDurableGrant(t *testing.T) {
	grants := store(t)
	f := newPreparationManagerFixture(t, grants)
	f.approve(t, "persistent")
	original := f.snapshot.Proposal.Fingerprint
	publish := func(AuthorizedRoot, []Registration) error { return errors.New("synthetic publication failure") }
	if err := f.manager.StartPreparation(f.snapshot.Context, f.discover, publish); err != nil {
		t.Fatal(err)
	}
	_ = nextStart(t, f.starts).process.Terminate()
	waitPreparationWorkers(t, f.manager)
	snapshot, err := f.manager.GetSnapshot(f.snapshot.Context)
	if err != nil || snapshot.Preparation == nil || snapshot.Preparation.State != "failed" {
		t.Fatalf("publication failure: %+v %v", snapshot, err)
	}
	found, err := hasGrant(grants, f.root.Project(), original)
	if err != nil || !found {
		t.Fatalf("publication failure moved original grant: %v %v", found, err)
	}
}

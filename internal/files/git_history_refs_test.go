package files

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHistoryBranches(t *testing.T) {
	root, git := blameRepo(t)
	ctx := context.Background()
	target := GitHistoryTarget{Dir: root, Repo: "."}
	refs, err := GitHistoryBranches(ctx, target)
	if err != nil || !refs.Unborn {
		t.Fatalf("unborn: %+v %v", refs, err)
	}
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "initial")
	branch := git("branch", "--show-current")
	git("update-ref", "refs/remotes/origin/"+branch, "HEAD")
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+branch)
	before := git("status", "--porcelain=v1")
	refs, err = GitHistoryBranches(ctx, target)
	if err != nil || refs.Unborn || refs.Detached || len(refs.Refs) != 2 || refs.Head != git("rev-parse", "HEAD") {
		t.Fatalf("refs: %+v %v", refs, err)
	}
	if git("status", "--porcelain=v1") != before || git("branch", "--show-current") != branch {
		t.Fatal("history changed working state")
	}
	git("checkout", "--detach", "HEAD")
	refs, err = GitHistoryBranches(ctx, target)
	if err != nil || !refs.Detached {
		t.Fatalf("detached: %+v %v", refs, err)
	}
	// A nested ref under the unborn branch name is a prefix match, not the branch.
	git("update-ref", "refs/heads/next/child", "HEAD")
	git("symbolic-ref", "HEAD", "refs/heads/next")
	refs, err = GitHistoryBranches(ctx, target)
	if err != nil || !refs.Unborn {
		t.Fatalf("unborn beside nested ref: %+v %v", refs, err)
	}
	git("symbolic-ref", "HEAD", "refs/heads/"+branch)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = GitHistoryBranches(canceled, target); err == nil {
		t.Fatal("canceled read succeeded")
	}
}

func TestGitHistoryTags(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "initial")
	commit := git("rev-parse", "HEAD")
	git("branch", "same")
	git("tag", "same")
	git("tag", "-a", "annotated", "-m", "release")
	git("tag", "-a", "nested", "annotated", "-m", "nested release")
	git("tag", "blob", "HEAD:file")
	git("tag", "tree", "HEAD^{tree}")
	target := GitHistoryTarget{Dir: root, Repo: "."}
	refs, err := GitHistoryBranches(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if refs.HeadRef != "refs/heads/"+git("branch", "--show-current") {
		t.Fatalf("head ref: %q", refs.HeadRef)
	}
	tags := 0
	for _, ref := range refs.Refs {
		if ref.Kind != "tag" {
			continue
		}
		tags++
		if ref.OID != git("rev-parse", ref.Ref) {
			t.Fatalf("raw tag identity: %+v", ref)
		}
		if ref.Name == "blob" || ref.Name == "tree" {
			if ref.Commit != "" {
				t.Fatalf("noncommit tag: %+v", ref)
			}
			continue
		}
		if ref.Commit != commit {
			t.Fatalf("peeled tag: %+v", ref)
		}
		page, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: target, Ref: ref.Ref, Limit: 50})
		if err != nil || page.Tip != commit {
			t.Fatalf("tag history: %+v %v", page, err)
		}
	}
	if tags != 5 {
		t.Fatalf("tag count: %d", tags)
	}
}

func TestHistoryRunnerEnvironmentAndLockCancellation(t *testing.T) {
	root, _ := blameRepo(t)
	err := withGitHistory(context.Background(), GitHistoryTarget{Dir: root, Repo: "."}, func(w gitWriter, _ gitHistoryResolvedTarget) error {
		for _, expected := range []string{"GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1"} {
			found := false
			for _, value := range w.env {
				if value == expected {
					found = true
				}
			}
			if !found {
				t.Fatalf("runner missing %s", expected)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	common, err := filepath.EvalSymlinks(filepath.Join(root, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockGitWrite(context.Background(), common)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitHistoryBranches(ctx, GitHistoryTarget{Dir: root, Repo: "."}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestHistorySHA256(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", "--object-format=sha256", root)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("installed Git lacks SHA256: %s", data)
	}
	for _, args := range [][]string{{"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit: %v %s", err, data)
		}
	}
	refs, err := GitHistoryBranches(context.Background(), GitHistoryTarget{Dir: root, Repo: "."})
	if err != nil || len(refs.Head) != 64 {
		t.Fatalf("SHA256: %+v %v", refs, err)
	}
}

func TestParseHistoryRefs(t *testing.T) {
	oid := strings.Repeat("a", 40)
	data := "refs/heads/main\x00" + oid + "\x00*\x00\nrefs/remotes/origin/main\x00" + oid + "\x00 \x00\nrefs/remotes/origin/HEAD\x00" + oid + "\x00 \x00refs/remotes/origin/main\n"
	got, err := parseHistoryRefs([]byte(data), 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 2 || !got.Refs[0].Current || !got.Refs[1].Remote {
		t.Fatalf("refs: %+v", got)
	}
	for _, invalid := range []string{"bad", "refs/notes/v1\x00" + oid + "\x00 \x00\n", "refs/heads/main\x00bad\x00 \x00\n"} {
		if _, err := parseHistoryRefs([]byte(invalid), 40); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	many := strings.Repeat("refs/heads/main\x00"+oid+"\x00 \x00\n", MaxGitBranches+1)
	got, err = parseHistoryRefs([]byte(many), 40)
	if err != nil || len(got.Refs) != MaxGitBranches || !got.Truncated {
		t.Fatalf("cap: %+v %v", got, err)
	}
	if !validHistoryOID(strings.Repeat("f", 64), 64) || validHistoryOID(oid, 64) || validHistoryOID(strings.Repeat("z", 40), 40) {
		t.Fatal("object format validation")
	}
}

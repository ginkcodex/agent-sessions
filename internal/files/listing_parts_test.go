package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestListingPartsMatchList(t *testing.T) {
	for _, count := range []int{8, MaxFiles + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			root := listingTree(t, count)
			for _, dir := range []string{root, filepath.Join(root, "nested")} {
				var mu sync.Mutex
				calls := map[string]int{}
				ctx := context.WithValue(context.Background(), gitInvocationKey{}, func(command string) {
					mu.Lock()
					calls[command]++
					mu.Unlock()
				})
				paths, err := ListPaths(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				if paths.Status != nil || calls["status"] != 0 || calls["rev-parse"] != 0 || calls["ls-files"] != 2 {
					t.Fatalf("path work: %v", calls)
				}
				before := calls["check-ignore"]
				paths.Status = ListingStatus(ctx, paths)
				if calls["status"] != 1 || calls["rev-parse"] != 1 || calls["ls-files"] != 2 || calls["check-ignore"] != before {
					t.Fatalf("status work: %v", calls)
				}
				want, err := List(context.Background(), dir)
				if err != nil || !reflect.DeepEqual(paths, want) {
					t.Fatalf("parts differ: %v\ngot %+v\nwant %+v", err, paths, want)
				}
			}
		})
	}
}

func TestWalkPartsMatchList(t *testing.T) {
	repo := listingTree(t, 2)
	root := t.TempDir()
	if err := os.Rename(repo, filepath.Join(root, "repo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := ListPaths(context.Background(), root)
	if err != nil || paths.Git || paths.Status != nil || len(paths.Repositories) != 1 {
		t.Fatalf("walk paths: %+v %v", paths, err)
	}
	paths.Status = ListingStatus(context.Background(), paths)
	want, err := List(context.Background(), root)
	if err != nil || !reflect.DeepEqual(paths, want) {
		t.Fatalf("walk parts differ: %v\ngot %+v\nwant %+v", err, paths, want)
	}
	if paths.Status["repo/deleted.txt"] != Deleted {
		t.Fatal("nested deletion missing")
	}
}

func TestListPathsErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(root, "absent")} {
		if _, err := ListPaths(context.Background(), path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListPaths(ctx, root); err != context.Canceled {
		t.Fatalf("canceled walk: %v", err)
	}
}

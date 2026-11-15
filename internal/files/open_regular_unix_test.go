//go:build unix

package files

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSpecialFileReadsDoNotBlock(t *testing.T) {
	root, _ := blameRepo(t)
	fifo := filepath.Join(root, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	socket, err := net.Listen("unix", filepath.Join(root, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := os.Symlink("fifo", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fifo", "link", "socket"} {
		for label, read := range map[string]func() error{
			"read":         func() error { _, err := Read(context.Background(), root, name); return err },
			"blame":        func() error { _, err := BlameFile(context.Background(), root, name, 1); return err },
			"working diff": func() error { _, err := readGitWorking(root, name); return err },
			"public diff": func() error {
				_, err := ReadGitDiff(context.Background(), EditPolicy{}, root, name, GitDiffHead)
				return err
			},
			"external":  func() error { _, err := ReadExternal(EditPolicy{}, filepath.Join(root, name)); return err },
			"download":  func() error { _, err := WriteDownload(context.Background(), root, name, io.Discard); return err },
			"directory": func() error { _, err := ListDirectory(context.Background(), root, name); return err },
		} {
			t.Run(name+"/"+label, func(t *testing.T) {
				done := make(chan error, 1)
				go func() { done <- read() }()
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("special file accepted")
					}
				case <-time.After(time.Second):
					t.Fatal("special-file read blocked")
				}
			})
		}
		if _, ok := searchFile(root, name, matcher{}); ok {
			t.Fatalf("search accepted %s", name)
		}
	}
	listing, err := walkPaths(context.Background(), root, nil)
	if err != nil || len(listing.Files) != 0 {
		t.Fatalf("walk: %+v %v", listing, err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "sub", "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ListDirectory(context.Background(), root, "sub")
	if err != nil || len(entries.Files) != 0 {
		t.Fatalf("directory: %+v %v", entries, err)
	}
}

func TestRegularOpenSurvivesSymlinkAndFIFOSwaps(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			for _, kind := range []string{"file", "fifo", "link"} {
				select {
				case <-stop:
					return
				default:
				}
				next := filepath.Join(root, "next")
				switch kind {
				case "file":
					_ = os.WriteFile(next, []byte("safe"), 0o600)
				case "fifo":
					_ = unix.Mkfifo(next, 0o600)
				case "link":
					_ = os.Symlink(filepath.Join(outside, "secret"), next)
				}
				_ = os.Rename(next, target)
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	done := make(chan error, 1)
	go func() {
		for range 1000 {
			f, _, err := openRegular(root, target)
			if err != nil {
				continue
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				done <- err
				return
			}
			if !slices.Equal(data, []byte("safe")) {
				done <- ErrOutsideRoot
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("regular-file swap blocked")
	}
}

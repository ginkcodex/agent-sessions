package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

func TestSetValue(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty file", "", "[terminal]\napp = \"kitty\"\n"},
		{
			"other table only",
			"[manage]\nenabled = true\n",
			"[manage]\nenabled = true\n\n[terminal]\napp = \"kitty\"\n",
		},
		{
			"replace in place, keep other keys and comments",
			"# mine\n[terminal]\ncommand = [\"x\"] # old key\napp = \"konsole\"\n\n[manage]\nenabled = false\n",
			"# mine\n[terminal]\ncommand = [\"x\"] # old key\napp = \"kitty\"\n\n[manage]\nenabled = false\n",
		},
		{
			"add to existing table before the next one",
			"[terminal]\ncommand = []\n\n[manage]\nenabled = true\n",
			"[terminal]\ncommand = []\napp = \"kitty\"\n\n[manage]\nenabled = true\n",
		},
		{
			"last table without trailing newline",
			"[manage]\nenabled = true\n[terminal]",
			"[manage]\nenabled = true\n[terminal]\napp = \"kitty\"\n",
		},
		{
			"duplicate keys collapse to one",
			"[terminal]\napp = \"a\"\napp = \"b\"\n",
			"[terminal]\napp = \"kitty\"\n",
		},
		{
			"same key in another table is untouched",
			"[other]\napp = \"x\"\n",
			"[other]\napp = \"x\"\n\n[terminal]\napp = \"kitty\"\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(SetValue([]byte(c.in), "terminal", "app", `"kitty"`))
			if got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
			if v, ok := Value([]byte(got), "terminal", "app"); !ok || v != "kitty" {
				t.Errorf("Value = %q, %v", v, ok)
			}
		})
	}
}

func TestValue(t *testing.T) {
	data := []byte("[manage]\napp = \"no\"\n[terminal]\n  app = 'ghostty' # chosen\nbare = foot ; note\n")
	for key, want := range map[string]string{"app": "ghostty", "bare": "foot"} {
		if v, ok := Value(data, "terminal", key); !ok || v != want {
			t.Errorf("%s = %q, %v; want %q", key, v, ok, want)
		}
	}
	if _, ok := Value(data, "terminal", "missing"); ok {
		t.Error("missing key found")
	}
	if _, ok := Value(nil, "terminal", "app"); ok {
		t.Error("key found in empty data")
	}
}

func TestUpdateKeepsConcurrentChanges(t *testing.T) {
	path := filepath.Join(platform.TempDir(t), "cfg", "config.toml")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i)
			if err := Update(path, func(data []byte) []byte {
				return SetValue(data, "t", key, "true")
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		if v, ok := Value(data, "t", fmt.Sprintf("k%d", i)); !ok || v != "true" {
			t.Errorf("k%d lost:\n%s", i, data)
		}
	}
	if platform.ModeBits {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, %v", fi.Mode(), err)
		}
	}
}

func TestWriteAtomicSweepsStaleTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	stale := filepath.Join(dir, "config-111.tmp")
	fresh := filepath.Join(dir, "config-222.tmp")
	for _, name := range []string{stale, fresh} {
		if err := os.WriteFile(name, []byte("leftover"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A killed write's temp ages; a temp this write races is milliseconds old.
	old := time.Now().Add(-staleTempAge - time.Minute)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("[t]\nk = 1\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp survived: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh temp removed: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
		t.Errorf("config not written: %v, %q", err, data)
	}
}

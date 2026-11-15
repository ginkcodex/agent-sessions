package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

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

func TestStringRoundTrip(t *testing.T) {
	for _, value := range []string{
		"", `C:\keys\secret`, `a\"b#;c`, "\x00\x01\x07\b\t\n\v\f\r\x1f\x7f", "Tiếng Việt 日本語 😀", `literal \x01 and \u263A`,
	} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			data := SetValue(nil, "translate", "api_key", Quote(value))
			for range 3 {
				got, ok := Value(data, "translate", "api_key")
				if !ok || got != value {
					t.Fatalf("Value = %q, %v; want %q", got, ok, value)
				}
				var parsed struct {
					Translate struct {
						APIKey string `toml:"api_key"`
					}
				}
				if _, err := toml.Decode(string(data), &parsed); err != nil || parsed.Translate.APIKey != value {
					t.Fatalf("TOML decode = %q, %v; want %q", parsed.Translate.APIKey, err, value)
				}
				data = SetValue(data, "translate", "api_key", Quote(got))
			}
		})
	}
}

func TestValueQuotedStrings(t *testing.T) {
	for _, tc := range []struct{ literal, want string }{
		{`"a\\b\"c#;d" # comment`, `a\b"c#;d`},
		{`'a\b"c#;d' # comment`, `a\b"c#;d`},
		{`"\u263A\U0001F600"`, "☺😀"},
	} {
		data := []byte("[t]\nk = " + tc.literal + "\n")
		if got, ok := Value(data, "t", "k"); !ok || got != tc.want {
			t.Errorf("%s: Value = %q, %v; want %q", tc.literal, got, ok, tc.want)
		}
	}
	for _, literal := range []string{`"unterminated`, `"bad\q"`} {
		if _, ok := Value([]byte("[t]\nk = "+literal+"\n"), "t", "k"); ok {
			t.Errorf("accepted invalid string %s", literal)
		}
	}
}

func TestUpdateKeepsLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	long := strings.Repeat("x", 128*1024)
	original := "# " + long + "\n[other]\nlong = '" + long + "'\n\n[translate]\napi_key = '" + long + "'\n# after long lines\n[terminal]\napp = 'kitty'\n\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := Value([]byte(original), "translate", "api_key"); !ok || got != long {
		t.Fatalf("long value: length %d, found %v", len(got), ok)
	}
	if err := Update(path, func(data []byte) []byte {
		return SetValue(data, "translate", "api_key", Quote("new"))
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(original, "api_key = '"+long+"'", `api_key = "new"`, 1)
	if string(data) != want {
		t.Fatalf("rewrite lost or changed unrelated lines: got %d bytes, want %d", len(data), len(want))
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

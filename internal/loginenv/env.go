// Package loginenv reads the environment of the user's interactive login
// shell: the PATH and variables a terminal app would have. An app started
// from a desktop launcher, or a server started through a non-interactive
// shell, lacks what the shell's startup files add there, such as the
// directories of version managers (mise, nvm, pyenv) that are activated
// only for interactive shells.
package loginenv

import (
	"bytes"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// timeout bounds the login shell Env runs.
const timeout = 10 * time.Second

// marker separates what the shell's startup files print from env's output.
const marker = "__AGENT_SESSIONS_ENV__"

// maxOutput bounds what is kept of the shell's output.
const maxOutput = 1 << 20

var (
	once sync.Once
	env  []string
)

// Env is the environment of the user's interactive login shell, started in
// the home directory. It is read once per process; on Windows, or when the
// shell fails, it is the process's own environment.
func Env() []string {
	once.Do(func() {
		env = os.Environ()
		if runtime.GOOS == "windows" {
			return
		}
		home, _ := os.UserHomeDir()
		if e := read(Shell(), home, timeout); len(e) > 0 {
			env = e
		}
	})
	return slices.Clone(env)
}

// parse reads `env -0` output after marker's line.
func parse(out []byte) []string {
	i := bytes.Index(out, []byte(marker+"\n"))
	if i < 0 {
		return nil
	}
	var env []string
	for _, entry := range strings.Split(string(out[i+len(marker)+1:]), "\x00") {
		if k, _, ok := strings.Cut(entry, "="); ok && k != "" {
			env = append(env, entry)
		}
	}
	return env
}

// boundedBuffer keeps the first maxOutput bytes written to it.
type boundedBuffer struct{ buf bytes.Buffer }

func (w *boundedBuffer) Write(p []byte) (int, error) {
	if room := maxOutput - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

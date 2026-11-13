package update

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// How a Linux install gets its updates. The toast tells the user what to run.
const (
	// InstallAptRepo: apt lists this project's repository.
	InstallAptRepo = "apt-repo"
	// InstallDnfRepo: dnf lists this project's repository.
	InstallDnfRepo = "dnf-repo"
	// InstallAUR: an Arch system, which gets the app from the AUR.
	InstallAUR = "aur"
	// InstallManual: a package installed by hand, or anything else.
	InstallManual = "manual"
)

// RepoHost serves the apt and dnf repositories.
const RepoHost = "agent-sessions.ginklab.cv"

// LinuxInstall works out how the Linux system under root ("/" outside tests)
// gets this app's updates. A repository counts only when a source that is
// not commented out or disabled names RepoHost.
func LinuxInstall(root string) string {
	etc := filepath.Join(root, "etc")
	apt := []string{filepath.Join(etc, "apt", "sources.list")}
	apt = append(apt, listDir(filepath.Join(etc, "apt", "sources.list.d"))...)
	for _, path := range apt {
		if names(path, aptSourceNames) {
			return InstallAptRepo
		}
	}
	for _, path := range listDir(filepath.Join(etc, "yum.repos.d")) {
		if strings.HasSuffix(path, ".repo") && names(path, yumRepoNames) {
			return InstallDnfRepo
		}
	}
	if _, err := os.Stat(filepath.Join(etc, "arch-release")); err == nil {
		return InstallAUR
	}
	return InstallManual
}

// listDir is the regular files in dir, or none.
func listDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

// names reads at most 1 MB of path and asks match whether it names RepoHost.
func names(path string, match func(name string, data []byte) bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil || !bytes.Contains(data, []byte(RepoHost)) {
		return false
	}
	return match(filepath.Base(path), data)
}

// aptSourceNames reads a one-line-style .list file (sources.list included),
// or a deb822 .sources file, whose paragraphs may say "Enabled: no".
func aptSourceNames(name string, data []byte) bool {
	switch {
	case strings.HasSuffix(name, ".sources"):
		for _, para := range paragraphs(data) {
			uris, enabled := para["uris"], para["enabled"]
			if strings.Contains(uris, RepoHost) && !strings.EqualFold(enabled, "no") {
				return true
			}
		}
		return false
	case name == "sources.list" || strings.HasSuffix(name, ".list"):
		// do-release-upgrade disables a third-party source by commenting it
		// out.
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "deb") && strings.Contains(stripComment(line), RepoHost) {
				return true
			}
		}
	}
	return false
}

// paragraphs splits deb822 data into lower-cased fields, one map per
// paragraph. Continuation lines join their field.
func paragraphs(data []byte) []map[string]string {
	var out []map[string]string
	cur := map[string]string{}
	last := ""
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
		}
		cur, last = map[string]string{}, ""
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "":
			flush()
		case strings.HasPrefix(line, "#"):
		case line[0] == ' ' || line[0] == '\t':
			if last != "" {
				cur[last] += " " + strings.TrimSpace(line)
			}
		default:
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			last = strings.ToLower(strings.TrimSpace(key))
			cur[last] = strings.TrimSpace(value)
		}
	}
	flush()
	return out
}

// yumRepoNames reads an ini-style .repo file: a section whose baseurl,
// mirrorlist or metalink names RepoHost, and which is not enabled=0.
func yumRepoNames(_ string, data []byte) bool {
	type section struct {
		ours    bool
		enabled bool
	}
	var secs []section
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") {
			secs = append(secs, section{enabled: true})
			continue
		}
		if len(secs) == 0 {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		s := &secs[len(secs)-1]
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "baseurl", "mirrorlist", "metalink":
			if strings.Contains(value, RepoHost) {
				s.ours = true
			}
		case "enabled":
			v := strings.ToLower(strings.TrimSpace(value))
			s.enabled = v != "0" && v != "false" && v != "no"
		}
	}
	for _, s := range secs {
		if s.ours && s.enabled {
			return true
		}
	}
	return false
}

func stripComment(line string) string {
	before, _, _ := strings.Cut(line, "#")
	return before
}

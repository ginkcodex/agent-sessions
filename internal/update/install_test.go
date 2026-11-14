package update

import (
	"os"
	"path/filepath"
	"testing"
)

// etcRoot writes files, by path under root, and returns root.
func etcRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	ourDeb   = "deb [signed-by=/usr/share/keyrings/agent-sessions.gpg] https://agent-sessions.ginklab.cv/apt stable main\n"
	otherDeb = "deb [signed-by=/usr/share/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu noble stable\n"
	ourRepo  = "[agent-sessions]\nname=Agent Sessions\nbaseurl=https://agent-sessions.ginklab.cv/rpm\nenabled=1\ngpgcheck=1\n"
)

func TestLinuxInstall(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"nothing", nil, InstallManual},
		{"apt list", map[string]string{"etc/apt/sources.list.d/agent-sessions.list": ourDeb}, InstallAptRepo},
		{"apt line in sources.list", map[string]string{"etc/apt/sources.list": otherDeb + ourDeb}, InstallAptRepo},
		{"apt list under another name", map[string]string{"etc/apt/sources.list.d/extra.list": ourDeb}, InstallAptRepo},
		{"apt deb822", map[string]string{"etc/apt/sources.list.d/agent-sessions.sources": "Types: deb\nURIs: https://agent-sessions.ginklab.cv/apt\nSuites: stable\nComponents: main\nSigned-By: /usr/share/keyrings/agent-sessions.gpg\n"}, InstallAptRepo},
		{"apt deb822 disabled", map[string]string{"etc/apt/sources.list.d/agent-sessions.sources": "Types: deb\nURIs: https://agent-sessions.ginklab.cv/apt\nSuites: stable\nComponents: main\nEnabled: no\n"}, InstallManual},
		{"apt deb822, ours in a second paragraph", map[string]string{"etc/apt/sources.list.d/many.sources": "Types: deb\nURIs: https://download.docker.com/linux/ubuntu\nSuites: noble\n\nTypes: deb\nURIs:\n https://agent-sessions.ginklab.cv/apt\nSuites: stable\n"}, InstallAptRepo},
		{"apt commented out on upgrade", map[string]string{"etc/apt/sources.list.d/agent-sessions.list": "# " + ourDeb[:len(ourDeb)-1] + " # disabled on upgrade to noble\n"}, InstallManual},
		{"apt another repository", map[string]string{"etc/apt/sources.list.d/docker.list": otherDeb}, InstallManual},
		{"apt host only in a comment", map[string]string{"etc/apt/sources.list.d/docker.list": otherDeb[:len(otherDeb)-1] + " # see agent-sessions.ginklab.cv\n"}, InstallManual},
		{"apt file not a .list", map[string]string{"etc/apt/sources.list.d/agent-sessions.list.save": ourDeb}, InstallManual},
		{"dnf repo", map[string]string{"etc/yum.repos.d/agent-sessions.repo": ourRepo}, InstallDnfRepo},
		{"dnf disabled", map[string]string{"etc/yum.repos.d/agent-sessions.repo": "[agent-sessions]\nbaseurl=https://agent-sessions.ginklab.cv/rpm\nenabled=0\n"}, InstallManual},
		{"dnf another repository", map[string]string{"etc/yum.repos.d/docker-ce.repo": "[docker-ce-stable]\nbaseurl=https://download.docker.com/linux/fedora/$releasever/$basearch/stable\nenabled=1\n"}, InstallManual},
		{"dnf host only in a name", map[string]string{"etc/yum.repos.d/mirror.repo": "[mirror]\nname=not agent-sessions.ginklab.cv\nbaseurl=https://mirror.example/rpm\n"}, InstallManual},
		{"dnf file not a .repo", map[string]string{"etc/yum.repos.d/agent-sessions.repo.rpmsave": ourRepo}, InstallManual},
		{"arch", map[string]string{"etc/arch-release": ""}, InstallAUR},
		{"arch with our repository too", map[string]string{"etc/arch-release": "", "etc/apt/sources.list.d/agent-sessions.list": ourDeb}, InstallAptRepo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LinuxInstall(etcRoot(t, tc.files)); got != tc.want {
				t.Fatalf("LinuxInstall = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestManagerStatusSaysHowTheAppIsInstalled(t *testing.T) {
	m := &Manager{Install: func() string { return InstallDnfRepo }}
	if got := m.Status().Install; got != InstallDnfRepo {
		t.Fatalf("Status().Install = %q", got)
	}
	if got := (&Manager{}).Status().Install; got != "" {
		t.Fatalf("without Install, Status().Install = %q", got)
	}
}

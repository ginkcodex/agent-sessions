package main

import (
	"reflect"
	"strings"
	"testing"
)

type fakeWizard struct {
	base    *fakePlatform
	titles  []string
	prompts []wizardPrompt
	answers []bool
}

func (v *fakeWizard) Show(title, body string) { v.titles = append(v.titles, title) }
func (v *fakeWizard) Ask(kind wizardPrompt, text string) bool {
	v.prompts = append(v.prompts, kind)
	if kind == promptConsent && len(transactions(v.base.events)) != 0 {
		panic("transaction before consent")
	}
	if len(v.answers) == 0 {
		return false
	}
	yes := v.answers[0]
	v.answers = v.answers[1:]
	return yes
}

func TestGuidedSetupStages(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		p := newFake()
		if upgrade {
			p.products = []product{oldProduct()}
		}
		v := &fakeWizard{base: p, answers: []bool{true}}
		out := runSetup(&guidedPlatform{platform: p, ui: v})
		if !out.Success {
			t.Fatal(out)
		}
		want := []string{"Preparing setup"}
		if upgrade {
			want = append(want, "Removing the previous installation", "Verifying installation state")
		}
		want = append(want, "Installing Agent Sessions", "Verifying installation state")
		if !reflect.DeepEqual(v.titles, want) {
			t.Fatalf("stages %v; want %v", v.titles, want)
		}
		if !reflect.DeepEqual(v.prompts, []wizardPrompt{promptConsent}) {
			t.Fatal(v.prompts)
		}
	}
}

func TestGuidedCancelBeforeRemoval(t *testing.T) {
	p := newFake()
	p.products = []product{oldProduct()}
	v := &fakeWizard{base: p, answers: []bool{false}}
	out := runSetup(&guidedPlatform{platform: p, ui: v})
	if out.Status != 1602 || len(transactions(p.events)) != 0 {
		t.Fatalf("%+v %v", out, p.events)
	}
}

func TestGuidedCloseApp(t *testing.T) {
	for _, retry := range []bool{false, true} {
		p := newFake()
		p.running = []bool{true, false}
		v := &fakeWizard{base: p, answers: []bool{true, retry}}
		out := runSetup(&guidedPlatform{platform: p, ui: v})
		if out.Success != retry || !reflect.DeepEqual(v.prompts, []wizardPrompt{promptConsent, promptClose}) {
			t.Fatalf("%+v %v", out, v.prompts)
		}
		if !retry && (out.Status != 1602 || len(transactions(p.events)) != 0) {
			t.Fatal(out)
		}
	}
}

func TestGuidedFailureAndRestart(t *testing.T) {
	for _, status := range []uint32{1603, 3010, 1641} {
		p := newFake()
		p.products = []product{oldProduct()}
		p.statuses = []uint32{status}
		v := &fakeWizard{base: p, answers: []bool{true}}
		out := runSetup(&guidedPlatform{platform: p, ui: v})
		if out.Success || out.Status != status || len(transactions(p.events)) != 1 {
			t.Fatalf("status %d: %+v %v", status, out, p.events)
		}
	}
}

func TestWizardResults(t *testing.T) {
	for _, tc := range []struct {
		out   outcome
		title string
		code  int
	}{
		{outcome{Success: true}, "Setup completed", 0},
		{outcome{Success: true, Status: 3010}, "Windows restart required", 3010},
		{outcome{Status: 1641}, "Windows restart required", 1641},
		{outcome{Status: 1602}, "Setup cancelled", 1602},
		{outcome{}, "Setup could not complete", 1},
		{outcome{Status: 1603}, "Setup could not complete", 1603},
	} {
		if got := resultTitle(tc.out); got != tc.title {
			t.Fatal(got)
		}
		if got := exitCode(tc.out); got != tc.code {
			t.Fatal(got)
		}
	}
	out := outcome{Message: "An error", Operation: "install", Status: 1603, Payload: payload{Directory: "C:\\long recovery path"}}
	text := finishText(out)
	for _, part := range []string{out.Message, out.Operation, "1603", out.Payload.Directory, "payload.msi", "have not been deleted"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %s", part, text)
		}
	}
	out.Success = true
	if strings.Contains(finishText(out), "If the old app") {
		t.Fatal("recovery warning on success")
	}
}

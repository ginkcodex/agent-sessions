package main

import (
	"testing"
	"time"
)

func TestWizardWorkerStartsOnlyOnce(t *testing.T) {
	s := wizardNavigation{page: wizardWelcome}
	if !s.startWorker() || s.page != wizardWorking {
		t.Fatal("welcome must start worker")
	}
	if s.startWorker() {
		t.Fatal("duplicate Next started another worker")
	}
	s.page = wizardQuestion
	if !s.canBack(true, promptConsent) {
		t.Fatal("consent must permit Back")
	}
	s.page = wizardWelcome
	if s.startWorker() {
		t.Fatal("Back then Next restarted worker")
	}
}

func TestWizardSafeNavigation(t *testing.T) {
	now := time.Now()
	s := wizardNavigation{page: wizardQuestion, started: true, nextAllowed: now.Add(time.Second)}
	if s.canAdvance(now) {
		t.Fatal("double-click must not authorize consent")
	}
	if !s.canAdvance(now.Add(time.Second)) {
		t.Fatal("deliberate Install must be enabled")
	}
	if !s.canCancel() {
		t.Fatal("pending decision must allow cancellation")
	}
	if s.canBack(true, promptClose) || s.canBack(false, promptConsent) {
		t.Fatal("only pending consent allows Back")
	}
	s.page = wizardWorking
	if s.canCancel() || s.canAdvance(now.Add(time.Hour)) || s.canBack(true, promptConsent) {
		t.Fatal("active work must disable navigation")
	}
	for _, page := range []int{wizardWelcome, wizardQuestion, wizardWorking} {
		s.page = page
		if s.canEndSession() {
			t.Fatal("session shutdown must not abandon started worker")
		}
	}
	s.finished = true
	if !s.canEndSession() {
		t.Fatal("finished worker must allow session shutdown")
	}
	s.page = wizardResult
	if !s.canAdvance(now) || !s.canCancel() {
		t.Fatal("result must permit Finish/close")
	}
}

package main

import "time"

const (
	wizardWelcome = iota
	wizardWorking
	wizardQuestion
	wizardResult
)

// wizardNavigation owns the view's non-platform navigation rules. In particular,
// returning to welcome never resets started: Next resumes a pending consent,
// rather than starting another preflight/transaction worker.
type wizardNavigation struct {
	page              int
	started, finished bool
	nextAllowed       time.Time
}

func (s *wizardNavigation) startWorker() bool {
	if s.page != wizardWelcome || s.started {
		return false
	}
	s.started = true
	s.page = wizardWorking
	return true
}
func (s *wizardNavigation) canBack(hasPending bool, kind wizardPrompt) bool {
	return s.page == wizardQuestion && hasPending && kind == promptConsent
}
func (s *wizardNavigation) canEndSession() bool { return !s.started || s.finished }

func (s *wizardNavigation) canCancel() bool {
	return s.page == wizardQuestion || s.page == wizardResult || s.page == wizardWelcome
}
func (s *wizardNavigation) canAdvance(now time.Time) bool {
	return s.page == wizardWelcome || s.page == wizardResult || (s.page == wizardQuestion && !now.Before(s.nextAllowed))
}

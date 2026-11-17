//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Foundation
#include <stdlib.h>
void textcheck_register_defaults(void);
int textcheck_default_value(const char *key);
*/
import "C"

import "unsafe"

// webTextCheckingDefaults are the user defaults WebKit's TextCheckerMac.mm
// reads before it falls back to the system settings (KD-06).
var webTextCheckingDefaults = []string{
	"WebContinuousSpellCheckingEnabled",
	"WebGrammarCheckingEnabled",
	"WebAutomaticSpellingCorrectionEnabled",
	"WebAutomaticTextReplacementEnabled",
	"WebAutomaticQuoteSubstitutionEnabled",
	"WebAutomaticDashSubstitutionEnabled",
	"WebAutomaticLinkDetectionEnabled",
	"WebSmartInsertDeleteEnabled",
	"WebSmartListsEnabled",
}

// WebKit reads these once, when the first web view is created, so register
// them before main runs. The registration domain is never saved, and a choice
// the user makes from a field's context menu still wins.
func init() { C.textcheck_register_defaults() }

// webTextCheckingDefault returns the effective value of one WebKit default,
// and false for set when no defaults domain has it.
func webTextCheckingDefault(key string) (on, set bool) {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	switch C.textcheck_default_value(k) {
	case -1:
		return false, false
	case 0:
		return false, true
	default:
		return true, true
	}
}

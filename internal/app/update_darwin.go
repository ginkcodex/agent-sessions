package app

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/update"
	"github.com/ginkcode/agent-sessions/internal/version"
)

func init() {
	canSelfUpdate = func() bool { return macInstall() != nil }
	applyUpdate = macApply
	prepareUpdate = macPrepare
}

var errNotUpdatable = errors.New("this install cannot update itself")

var macInstalled struct {
	once sync.Once
	mac  *update.Mac
}

// macInstall describes the running app when it can replace itself, else nil.
// It is worked out once: it runs codesign.
func macInstall() *update.Mac {
	macInstalled.once.Do(func() {
		if !update.Newer("999999.0.0", version.Current()) {
			return // a development build
		}
		exe, err := os.Executable()
		if err != nil {
			return
		}
		bundle := update.MacBundle(exe)
		if bundle == "" {
			return
		}
		mac := &update.Mac{Bundle: bundle}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mac.Eligible(ctx); err != nil {
			log.Printf("agent-sessions: automatic updates are off: %v", err)
			return
		}
		macInstalled.mac = mac
	})
	return macInstalled.mac
}

// macPrepare unpacks and checks the downloaded app beside the installed one.
func macPrepare(ctx context.Context, r update.Ready) error {
	mac := macInstall()
	if mac == nil {
		return errNotUpdatable
	}
	if _, err := mac.Stage(ctx, r); err != nil {
		return err
	}
	mac.Cleanup(r.Version)
	return nil
}

// macApply starts the helper that swaps the staged app in after this process
// exits.
func macApply(r update.Ready, relaunch bool) error {
	mac := macInstall()
	if mac == nil {
		return errNotUpdatable
	}
	return update.StartApply(mac, r, relaunch)
}

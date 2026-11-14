package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/chatattach"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// Every file here is made by the test.

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeFile(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestChatPickAttachments(t *testing.T) {
	a := NewAppWithService(chatTestService())
	log := useChatEngine(t, a)
	dir := t.TempDir()
	st, err := a.ChatStart(engine.ChatStartRequest{Agent: model.AgentClaude, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Without a desktop there is no picker.
	if _, err := a.ChatPickAttachments(st.ChatID); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("without a desktop: %v", err)
	}

	picked := []string{
		writeFile(t, dir, "notes.txt", []byte("hello")),
		writeFile(t, dir, "shot.png", tinyPNG(t)),
		writeFile(t, dir, "app.bin", []byte("MZ\x00\x01")),
	}
	var asked []OpenDialogOptions
	answer := picked
	a.ctx = withDesktop(context.Background(), Desktop{OpenFiles: func(_ context.Context, opts OpenDialogOptions) ([]string, error) {
		asked = append(asked, opts)
		return answer, nil
	}})

	res, err := a.ChatPickAttachments(st.ChatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0].DefaultDirectory != dir {
		t.Fatalf("dialog = %+v, want it to start in %s", asked, dir)
	}
	if len(res.Attachments) != 2 || res.Attachments[0].Name != "notes.txt" || res.Attachments[1].Name != "shot.png" {
		t.Fatalf("attachments = %+v", res.Attachments)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "app.bin" || !strings.Contains(res.Skipped[0].Reason, "only images and text files") {
		t.Fatalf("skipped = %+v", res.Skipped)
	}

	// The picked files reach the agent after the text, in order.
	if _, err := a.ChatPrompt(st.ChatID, "blocks", res.Attachments); err != nil {
		t.Fatal(err)
	}
	log.waitTurnEnd(t, st.ChatID)
	want := fmt.Sprintf("text blocks\nresource %s text/plain hello\nimage image/png %d",
		res.Attachments[0].URI, len(res.Attachments[1].Data))
	log.waitFor(t, "the blocks", func(u []engine.ChatUpdate, _ []engine.ChatExit) bool {
		return slices.ContainsFunc(u, func(u engine.ChatUpdate) bool {
			if u.Kind != engine.ChatSessionUpdate {
				return false
			}
			c, err := u.Update.AsChunk()
			return err == nil && c.Content.Text == want
		})
	})

	// A chat this computer does not run starts in the home directory.
	if _, err := a.ChatPickAttachments("no-such-chat"); err != nil {
		t.Fatal(err)
	}
	if asked[1].DefaultDirectory != dialogDefaultDir() {
		t.Errorf("unknown chat: dialog starts in %q", asked[1].DefaultDirectory)
	}

	// Cancelling attaches nothing, as empty lists.
	answer = nil
	res, err = a.ChatPickAttachments(st.ChatID)
	if err != nil || res.Attachments == nil || res.Skipped == nil || len(res.Attachments)+len(res.Skipped) != 0 {
		t.Fatalf("cancelled: %+v, %v", res, err)
	}

	// The agent runs in dir; Windows cannot remove a process's working
	// directory, so it must exit before the test's cleanup.
	if err := a.ChatClose(st.ChatID); err != nil {
		t.Fatal(err)
	}
	log.waitExit(t, st.ChatID)
}

func TestChatAttachFilesStopsAtTheLimit(t *testing.T) {
	a := NewAppWithService(chatTestService())
	dir := t.TempDir()
	var paths []string
	for i := range chatattach.MaxAttachments + 1 {
		paths = append(paths, writeFile(t, dir, fmt.Sprintf("f%02d.txt", i), []byte("x")))
	}
	paths = append(paths, filepath.Join(dir, "gone.txt"))
	res := a.ChatAttachFiles(paths)
	if len(res.Attachments) != chatattach.MaxAttachments || res.Attachments[9].Name != "f09.txt" {
		t.Fatalf("attachments = %d", len(res.Attachments))
	}
	if len(res.Skipped) != 2 || res.Skipped[0].Name != "f10.txt" || !strings.Contains(res.Skipped[0].Reason, "at most 10") ||
		res.Skipped[1].Name != "gone.txt" {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
	if res := a.ChatAttachFiles(nil); res.Attachments == nil || res.Skipped == nil {
		t.Fatalf("no files: %+v", res)
	}
}

func TestChatAttachImage(t *testing.T) {
	a := NewAppWithService(chatTestService())
	raw := tinyPNG(t)
	att, err := a.ChatAttachImage("", base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if att.Kind != chatattach.KindImage || att.Name != "Pasted image" || att.MimeType != "image/png" || att.URI != "" {
		t.Fatalf("got %+v", att)
	}
	if _, err := a.ChatAttachImage("clip", "%%%"); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("bad base64: %v", err)
	}
	_, err = a.ChatAttachImage("clip", base64.StdEncoding.EncodeToString([]byte("text")))
	var refused *chatattach.Error
	if !errors.As(err, &refused) || refused.Name != "clip" {
		t.Fatalf("not an image: %v", err)
	}
}

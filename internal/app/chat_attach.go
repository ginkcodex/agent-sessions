package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ginkcode/agent-sessions/internal/chatattach"
)

// ChatAttachResult is what the user picked or dropped, prepared to send:
// the files that could be attached, and the others with why not.
type ChatAttachResult struct {
	Attachments []chatattach.Attachment `json:"attachments"`
	Skipped     []ChatAttachSkip        `json:"skipped"`
}

// ChatAttachSkip is a file that could not be attached.
type ChatAttachSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ChatPickAttachments opens the file picker, for several files at once, and
// prepares the picked files to send. It starts in the chat's directory for
// a chat on this computer. Cancelling attaches nothing. The files are read
// here, so a chat on a connected host gets their content, not their paths.
func (a *App) ChatPickAttachments(chatID string) (ChatAttachResult, error) {
	paths, err := openFilesDialog(a.appCtx(), OpenDialogOptions{
		Title:            "Attach Files",
		DefaultDirectory: a.chatDialogDir(chatID),
	})
	if err != nil {
		return emptyAttachResult(), err
	}
	return attachFiles(paths), nil
}

// ChatAttachFiles prepares files dropped on the chat to send.
func (a *App) ChatAttachFiles(paths []string) ChatAttachResult {
	return attachFiles(paths)
}

// ChatAttachImage prepares a pasted image, its bytes in base64, to send.
func (a *App) ChatAttachImage(name, data string) (chatattach.Attachment, error) {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return chatattach.Attachment{}, errors.New("the pasted image could not be read")
	}
	return chatattach.FromImage(name, b)
}

// chatDialogDir is where the picker starts: the directory of a chat on this
// computer, or the home directory.
func (a *App) chatDialogDir(chatID string) string {
	if a.route().host == "" {
		if m, err := a.ownChats(); err == nil {
			for _, c := range m.Chats() {
				if c.ChatID == chatID && c.Dir != "" {
					return c.Dir
				}
			}
		}
	}
	return dialogDefaultDir()
}

func emptyAttachResult() ChatAttachResult {
	return ChatAttachResult{Attachments: []chatattach.Attachment{}, Skipped: []ChatAttachSkip{}}
}

// attachFiles reads each path as an attachment, in order, skipping those
// that cannot be attached and those past the most one message may carry.
func attachFiles(paths []string) ChatAttachResult {
	out := emptyAttachResult()
	for _, p := range paths {
		if len(out.Attachments) == chatattach.MaxAttachments {
			out.Skipped = append(out.Skipped, ChatAttachSkip{
				Name:   filepath.Base(p),
				Reason: fmt.Sprintf("a message can carry at most %d attachments", chatattach.MaxAttachments),
			})
			continue
		}
		att, err := chatattach.FromFile(p)
		if err != nil {
			skip := ChatAttachSkip{Name: filepath.Base(p), Reason: err.Error()}
			var e *chatattach.Error
			if errors.As(err, &e) {
				skip = ChatAttachSkip{Name: e.Name, Reason: e.Reason}
			}
			out.Skipped = append(out.Skipped, skip)
			continue
		}
		out.Attachments = append(out.Attachments, att)
	}
	return out
}

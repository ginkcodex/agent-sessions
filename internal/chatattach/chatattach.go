// Package chatattach turns files the user attaches to a chat message into
// ACP prompt blocks: images as image blocks, text files as embedded
// resources. Anything else is refused, with a reason that names the file.
//
// Files are read where the user picked them (the desktop app); the limits
// are checked again where the agent runs, since attachments may come from a
// connection peer.
package chatattach

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ginkcode/agent-sessions/internal/acp"
)

const (
	// MaxAttachments is the most attachments one message may carry.
	MaxAttachments = 10
	// MaxTotalBytes bounds a message's attachments as sent: base64 for
	// images, text for text files.
	MaxTotalBytes = 24 << 20
	// MaxFileBytes is the largest file read; larger ones are refused unread.
	MaxFileBytes = 20 << 20
	// MaxTextBytes is the largest text file attached.
	MaxTextBytes = 512 << 10
	// MaxImageBytes is the largest image sent: 5 MiB once in base64, the
	// Claude API's limit. Larger images are scaled down first.
	MaxImageBytes = 3 << 20 * 5 / 4
	// MaxImageEdge is the longest side of an image sent; longer ones are
	// scaled down. Models scale down anything larger anyway.
	MaxImageEdge = 2000
)

// Kinds of attachment.
const (
	KindImage = "image"
	KindText  = "text"
)

// Attachment is a file ready to send: an image's bytes or a text file's
// text.
type Attachment struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	// Size is the size of what is sent: the image's bytes, after any
	// scaling, or the text's.
	Size int64 `json:"size"`
	// Data is an image's bytes in base64.
	Data string `json:"data,omitempty"`
	// Text is a text file's content.
	Text string `json:"text,omitempty"`
	// URI is the file's file:// URL; empty for a pasted image.
	URI string `json:"uri,omitempty"`
}

// Error says why a file cannot be attached.
type Error struct {
	Name   string
	Reason string
}

func (e *Error) Error() string { return e.Name + ": " + e.Reason }

func refuse(name, format string, args ...any) error {
	return &Error{Name: name, Reason: fmt.Sprintf(format, args...)}
}

// ErrNoImages is returned by Blocks for an image to an agent that takes
// none.
var ErrNoImages = errors.New("the agent does not take images")

// FromFile reads the file at path as an attachment.
func FromFile(path string) (Attachment, error) {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return Attachment{}, refuse(name, "could not be read: %v", unwrapPath(err))
	case info.IsDir():
		return Attachment{}, refuse(name, "is a folder; attach the files in it instead")
	case !info.Mode().IsRegular():
		return Attachment{}, refuse(name, "is not a regular file")
	case info.Size() > MaxFileBytes:
		return Attachment{}, refuse(name, "is larger than %s", sizeText(MaxFileBytes))
	}
	f, err := os.Open(path)
	if err != nil {
		return Attachment{}, refuse(name, "could not be read: %v", unwrapPath(err))
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return Attachment{}, refuse(name, "could not be read: %v", unwrapPath(err))
	}
	if len(b) > MaxFileBytes {
		return Attachment{}, refuse(name, "is larger than %s", sizeText(MaxFileBytes))
	}
	uri := fileURI(path)
	if mt := imageType(b); mt != "" {
		return fromImage(name, uri, mt, b)
	}
	if !isText(b) {
		return Attachment{}, refuse(name, "is neither an image nor a text file; only images and text files can be attached")
	}
	if len(b) > MaxTextBytes {
		return Attachment{}, refuse(name, "is a text file larger than %s", sizeText(MaxTextBytes))
	}
	return Attachment{Kind: KindText, Name: name, MimeType: textType(name), Size: int64(len(b)), Text: string(b), URI: uri}, nil
}

// FromImage makes an attachment of an image's bytes, such as a pasted one.
func FromImage(name string, b []byte) (Attachment, error) {
	if name == "" {
		name = "Pasted image"
	}
	if len(b) > MaxFileBytes {
		return Attachment{}, refuse(name, "is larger than %s", sizeText(MaxFileBytes))
	}
	mt := imageType(b)
	if mt == "" {
		return Attachment{}, refuse(name, "is not a PNG, JPEG, GIF or WebP image")
	}
	return fromImage(name, "", mt, b)
}

func fromImage(name, uri, mimeType string, b []byte) (Attachment, error) {
	b, mimeType, err := fitImage(name, mimeType, b)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{
		Kind: KindImage, Name: name, MimeType: mimeType, Size: int64(len(b)),
		Data: base64.StdEncoding.EncodeToString(b), URI: uri,
	}, nil
}

// Check checks attachments from anywhere, such as a connection peer: their
// number, their total size, and that each is what it says.
func Check(atts []Attachment) error {
	if len(atts) > MaxAttachments {
		return fmt.Errorf("a message can carry at most %d attachments", MaxAttachments)
	}
	total := 0
	for _, a := range atts {
		name := a.Name
		if strings.TrimSpace(name) == "" {
			return errors.New("an attachment has no name")
		}
		switch a.Kind {
		case KindImage:
			b, err := base64.StdEncoding.DecodeString(a.Data)
			if err != nil || len(b) == 0 {
				return refuse(name, "is not an image in base64")
			}
			if mt := imageType(b); mt == "" || mt != a.MimeType {
				return refuse(name, "is not the %s image it claims to be", a.MimeType)
			}
			if len(b) > MaxImageBytes {
				return refuse(name, "is an image larger than %s", sizeText(MaxImageBytes))
			}
			total += len(a.Data)
		case KindText:
			if a.Data != "" || !isText([]byte(a.Text)) {
				return refuse(name, "is not a text file")
			}
			if len(a.Text) > MaxTextBytes {
				return refuse(name, "is a text file larger than %s", sizeText(MaxTextBytes))
			}
			total += len(a.Text)
		default:
			return refuse(name, "is an attachment of an unknown kind %q", a.Kind)
		}
	}
	if total > MaxTotalBytes {
		return fmt.Errorf("the attachments are larger than %s together", sizeText(MaxTotalBytes))
	}
	return nil
}

// Blocks makes a prompt: the text, when there is any, then one block per
// attachment in order. Text files go in as embedded resources, or as fenced
// text for an agent without embedded context. An image for an agent without
// image support is ErrNoImages.
func Blocks(text string, atts []Attachment, caps acp.PromptCapabilities) ([]acp.ContentBlock, error) {
	blocks := make([]acp.ContentBlock, 0, len(atts)+1)
	if text != "" {
		blocks = append(blocks, acp.TextBlock(text))
	}
	for _, a := range atts {
		switch a.Kind {
		case KindImage:
			if !caps.Image {
				return nil, fmt.Errorf("%w: %s", ErrNoImages, a.Name)
			}
			blocks = append(blocks, acp.ImageBlock(a.MimeType, a.Data))
		case KindText:
			if caps.EmbeddedContext {
				uri := a.URI
				if uri == "" {
					uri = "attachment:" + url.PathEscape(a.Name)
				}
				blocks = append(blocks, acp.ResourceBlock(acp.TextResource{URI: uri, MimeType: a.MimeType, Text: a.Text}))
			} else {
				blocks = append(blocks, acp.TextBlock(inlineText(a)))
			}
		default:
			return nil, refuse(a.Name, "is an attachment of an unknown kind %q", a.Kind)
		}
	}
	return blocks, nil
}

// inlineText is a text file as a prompt's text: its name, then its text in
// a fence longer than any backtick run in it.
func inlineText(a Attachment) string {
	fence := "```"
	for strings.Contains(a.Text, fence) {
		fence += "`"
	}
	body := a.Text
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return fmt.Sprintf("Attached file %s:\n\n%s\n%s%s", a.Name, fence, body, fence)
}

// imageType is the type of an image the chat can send, by its first bytes,
// or "".
func imageType(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

// isText reports whether b reads as text: UTF-8 without NUL bytes.
func isText(b []byte) bool {
	return utf8.Valid(b) && bytes.IndexByte(b, 0) < 0
}

// textType is a text file's type by its extension, text/plain when it has
// none that is text.
func textType(name string) string {
	t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if t == "" {
		return "text/plain"
	}
	mt, _, err := mime.ParseMediaType(t)
	if err != nil || strings.HasPrefix(mt, "image/") || strings.HasPrefix(mt, "audio/") || strings.HasPrefix(mt, "video/") {
		return "text/plain"
	}
	return mt
}

// fileURI is path's file:// URL, with a Windows drive as /C:/.
func fileURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// unwrapPath drops the path from an *os.PathError: the message already
// names the file.
func unwrapPath(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func sizeText(n int) string {
	if n%(1<<20) == 0 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KiB", n>>10)
}

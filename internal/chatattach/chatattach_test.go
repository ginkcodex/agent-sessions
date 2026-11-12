package chatattach

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/acp"
)

// All images here are generated; no file of the user's is read.

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// gradient is an image that compresses well.
func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, 255})
		}
	}
	return img
}

// noise is an image that does not compress.
func noise(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.Uint32())
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func write(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func decoded(t *testing.T, a Attachment) image.Config {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(a.Data)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(b)) != a.Size {
		t.Errorf("size %d, data has %d bytes", a.Size, len(b))
	}
	if len(b) > MaxImageBytes {
		t.Errorf("%d bytes, over the limit", len(b))
	}
	if got := imageType(b); got != a.MimeType {
		t.Errorf("bytes are %q, the attachment says %q", got, a.MimeType)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func refusedFor(t *testing.T, err error, name, reason string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if e.Name != name || !strings.Contains(e.Reason, reason) {
		t.Fatalf("got %q: %q, want %q: …%s…", e.Name, e.Reason, name, reason)
	}
}

func TestSmallImageGoesAsItIs(t *testing.T) {
	raw := pngBytes(t, gradient(40, 20))
	p := write(t, "shot.png", raw)
	a, err := FromFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindImage || a.Name != "shot.png" || a.MimeType != "image/png" || a.Text != "" {
		t.Fatalf("got %+v", a)
	}
	if a.Data != base64.StdEncoding.EncodeToString(raw) {
		t.Error("a small image should go unchanged")
	}
	if !strings.HasPrefix(a.URI, "file:///") || !strings.HasSuffix(a.URI, "/shot.png") {
		t.Errorf("uri %q", a.URI)
	}
}

func TestImageTypeComesFromTheBytes(t *testing.T) {
	// A PNG named .txt is an image; a text file named .png is text.
	a, err := FromFile(write(t, "notes.txt", pngBytes(t, gradient(4, 4))))
	if err != nil || a.Kind != KindImage || a.MimeType != "image/png" {
		t.Fatalf("got %+v, %v", a, err)
	}
	a, err = FromFile(write(t, "fake.png", []byte("not an image\n")))
	if err != nil || a.Kind != KindText {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func TestLongImageIsScaledToFit(t *testing.T) {
	a, err := FromImage("wide.png", pngBytes(t, gradient(3000, 1500)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := decoded(t, a)
	if cfg.Width != 2000 || cfg.Height != 1000 {
		t.Fatalf("scaled to %d×%d, want 2000×1000", cfg.Width, cfg.Height)
	}
	if a.MimeType != "image/png" {
		t.Errorf("a PNG that fits stays PNG, got %s", a.MimeType)
	}
	if a.URI != "" {
		t.Errorf("a pasted image has no uri, got %q", a.URI)
	}
}

func TestHeavyImageFallsBackToJPEG(t *testing.T) {
	raw := pngBytes(t, noise(1500, 1500))
	if len(raw) <= MaxImageBytes {
		t.Fatalf("the test image is only %d bytes", len(raw))
	}
	a, err := FromImage("noise.png", raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg := decoded(t, a)
	if a.MimeType != "image/jpeg" || cfg.Width != 1500 || cfg.Height != 1500 {
		t.Fatalf("got %s %d×%d", a.MimeType, cfg.Width, cfg.Height)
	}
}

func TestLargeJPEGStaysJPEG(t *testing.T) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, gradient(2400, 1200), nil); err != nil {
		t.Fatal(err)
	}
	a, err := FromImage("photo.jpg", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if cfg := decoded(t, a); a.MimeType != "image/jpeg" || cfg.Width != 2000 || cfg.Height != 1000 {
		t.Fatalf("got %s %d×%d", a.MimeType, cfg.Width, cfg.Height)
	}
}

func TestLargeGIFKeepsItsFirstFrameAsPNG(t *testing.T) {
	frame := func(c uint8) *image.Paletted {
		img := image.NewPaletted(image.Rect(0, 0, 2500, 100), palette.Plan9)
		for i := range img.Pix {
			img.Pix[i] = c
		}
		return img
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame(10), frame(200)}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	a, err := FromImage("anim.gif", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if cfg := decoded(t, a); a.MimeType != "image/png" || cfg.Width != 2000 || cfg.Height != 80 {
		t.Fatalf("got %s %d×%d", a.MimeType, cfg.Width, cfg.Height)
	}
	data, _ := base64.StdEncoding.DecodeString(a.Data)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want := color.RGBAModel.Convert(palette.Plan9[10]).(color.RGBA)
	if got := color.RGBAModel.Convert(img.At(5, 5)).(color.RGBA); got != want {
		t.Errorf("pixel %v, want the first frame's %v", got, want)
	}
}

func TestSmallGIFGoesAsItIs(t *testing.T) {
	var b bytes.Buffer
	if err := gif.Encode(&b, gradient(30, 30), nil); err != nil {
		t.Fatal(err)
	}
	a, err := FromImage("", b.Bytes())
	if err != nil || a.MimeType != "image/gif" || a.Name != "Pasted image" {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func webp(n int) []byte {
	return append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, n)...)
}

func TestWebPGoesAsItIsOrNotAtAll(t *testing.T) {
	a, err := FromImage("small.webp", webp(100))
	if err != nil || a.MimeType != "image/webp" || a.Kind != KindImage {
		t.Fatalf("got %+v, %v", a, err)
	}
	_, err = FromImage("big.webp", webp(MaxImageBytes))
	refusedFor(t, err, "big.webp", "WebP image larger than 3.75 MiB")
}

func TestScaledImageKeepsTransparencyAndFlattensOnWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	// The left half opaque red, the right half clear.
	for y := range 2 {
		for x := range 2 {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	scaled := scaleDown(img, 2, 1)
	if got := scaled.RGBAAt(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("left %v", got)
	}
	if got := scaled.RGBAAt(1, 0); got != (color.RGBA{}) {
		t.Errorf("right %v", got)
	}
	flat := flatten(scaled)
	if got := flat.RGBAAt(1, 0); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("clear on white is %v", got)
	}
	if got := flat.RGBAAt(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("opaque red is %v", got)
	}
}

func TestFitSize(t *testing.T) {
	for _, c := range []struct{ w, h, ww, wh int }{
		{100, 50, 100, 50},
		{2000, 2000, 2000, 2000},
		{4000, 3000, 2000, 1500},
		{1000, 3000, 667, 2000},
		{100000, 1, 2000, 1},
	} {
		if w, h := fitSize(c.w, c.h, 2000); w != c.ww || h != c.wh {
			t.Errorf("%d×%d → %d×%d, want %d×%d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}

func TestHugeImageIsRefusedBeforeDecoding(t *testing.T) {
	// Only a PNG header that claims 10000×10000 pixels: scaling would need
	// 400 MB, so it is refused from the header alone.
	chunk := []byte{'I', 'H', 'D', 'R', 0, 0, 0x27, 0x10, 0, 0, 0x27, 0x10, 8, 6, 0, 0, 0}
	b := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0d"), chunk...)
	b = binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(chunk))
	b = append(b, make([]byte, MaxImageBytes)...)
	_, err := FromImage("huge.png", b)
	refusedFor(t, err, "huge.png", "too large to scale down (10000×10000 pixels)")
}

func TestBrokenImageIsRefused(t *testing.T) {
	_, err := FromImage("broken.jpg", append([]byte("\xff\xd8\xff"), make([]byte, MaxImageBytes)...))
	refusedFor(t, err, "broken.jpg", "could not be read as an image")
	_, err = FromImage("paste", []byte("hello"))
	refusedFor(t, err, "paste", "not a PNG, JPEG, GIF or WebP image")
}

func TestTextFile(t *testing.T) {
	p := write(t, "main.go", []byte("package main\n"))
	a, err := FromFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindText || a.Text != "package main\n" || a.Size != 13 || a.Data != "" {
		t.Fatalf("got %+v", a)
	}
	if a.MimeType == "" || strings.Contains(a.MimeType, ";") {
		t.Errorf("mime type %q", a.MimeType)
	}
	for name, want := range map[string]string{"notes": "text/plain", "README.md": "text/markdown", "a.json": "application/json", "x.svg": "text/plain"} {
		got := textType(name)
		if name == "README.md" && got == "text/plain" {
			continue // not every system maps .md
		}
		if got != want {
			t.Errorf("textType(%q) = %q, want %q", name, got, want)
		}
	}
	// An empty file attaches, with no text.
	a, err = FromFile(write(t, "empty.txt", nil))
	if err != nil || a.Kind != KindText || a.Text != "" {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func TestRefusedFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := FromFile(dir)
	refusedFor(t, err, filepath.Base(dir), "is a folder")

	_, err = FromFile(filepath.Join(dir, "gone.txt"))
	refusedFor(t, err, "gone.txt", "could not be read")

	_, err = FromFile(write(t, "app.bin", []byte("MZ\x00\x01\x02")))
	refusedFor(t, err, "app.bin", "only images and text files")

	_, err = FromFile(write(t, "latin1.txt", []byte("caf\xe9\n")))
	refusedFor(t, err, "latin1.txt", "only images and text files")

	_, err = FromFile(write(t, "big.log", bytes.Repeat([]byte("a"), MaxTextBytes+1)))
	refusedFor(t, err, "big.log", "text file larger than 512 KiB")

	huge := filepath.Join(dir, "huge.iso")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = FromFile(huge)
	refusedFor(t, err, "huge.iso", "larger than 20 MiB")
}

func TestFileURIEscapes(t *testing.T) {
	p := write(t, "my notes #1.txt", []byte("x"))
	a, err := FromFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(a.URI, "/my%20notes%20%231.txt") {
		t.Errorf("uri %q", a.URI)
	}
}

func image64(t *testing.T) Attachment {
	t.Helper()
	a, err := FromImage("dot.png", pngBytes(t, gradient(2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCheck(t *testing.T) {
	img := image64(t)
	txt := Attachment{Kind: KindText, Name: "a.txt", MimeType: "text/plain", Text: "hi"}
	if err := Check([]Attachment{img, txt}); err != nil {
		t.Fatal(err)
	}
	if err := Check(nil); err != nil {
		t.Fatal(err)
	}
	many := make([]Attachment, MaxAttachments+1)
	for i := range many {
		many[i] = txt
	}
	if err := Check(many); err == nil || !strings.Contains(err.Error(), "at most 10") {
		t.Errorf("too many: %v", err)
	}

	// Five full-size images: each within the limit, too much together.
	big := Attachment{
		Kind: KindImage, Name: "big.png", MimeType: "image/png",
		Data: base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, MaxImageBytes-8)...)),
	}
	if err := Check([]Attachment{big}); err != nil {
		t.Fatal(err)
	}
	if err := Check([]Attachment{big, big, big, big, big}); err == nil || !strings.Contains(err.Error(), "larger than 24 MiB together") {
		t.Errorf("too large: %v", err)
	}

	for name, bad := range map[string]Attachment{
		"no name":        {Kind: KindText, Text: "x"},
		"unknown kind":   {Kind: "audio", Name: "a.wav"},
		"bad base64":     {Kind: KindImage, Name: "x.png", MimeType: "image/png", Data: "%%%"},
		"empty image":    {Kind: KindImage, Name: "x.png", MimeType: "image/png"},
		"wrong type":     {Kind: KindImage, Name: "x.png", MimeType: "image/jpeg", Data: img.Data},
		"not an image":   {Kind: KindImage, Name: "x.png", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("hello"))},
		"text with data": {Kind: KindText, Name: "x.txt", Text: "x", Data: img.Data},
		"binary text":    {Kind: KindText, Name: "x.txt", Text: "a\x00b"},
		"long text":      {Kind: KindText, Name: "x.txt", Text: strings.Repeat("a", MaxTextBytes+1)},
		"oversized image": {
			Kind: KindImage, Name: "x.png", MimeType: "image/png",
			Data: base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, MaxImageBytes)...)),
		},
	} {
		if err := Check([]Attachment{bad}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func blockJSON(t *testing.T, blocks []acp.ContentBlock) []string {
	t.Helper()
	out := make([]string, len(blocks))
	for i, b := range blocks {
		j, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = string(j)
	}
	return out
}

func TestBlocks(t *testing.T) {
	img := Attachment{Kind: KindImage, Name: "dot.png", MimeType: "image/png", Data: "AAAA"}
	txt := Attachment{Kind: KindText, Name: "a.go", MimeType: "text/x-go", Text: "package a\n", URI: "file:///p/a.go"}
	all := acp.PromptCapabilities{Image: true, EmbeddedContext: true}

	got := blockJSON(t, must(Blocks("look", []Attachment{txt, img}, all)))
	want := []string{
		`{"type":"text","text":"look"}`,
		`{"type":"resource","resource":{"uri":"file:///p/a.go","mimeType":"text/x-go","text":"package a\n"}}`,
		`{"type":"image","data":"AAAA","mimeType":"image/png"}`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// No text: only the attachments. A text file without a uri gets one.
	pasted := txt
	pasted.URI = ""
	pasted.Name = "my notes.txt"
	got = blockJSON(t, must(Blocks("", []Attachment{pasted}, all)))
	if len(got) != 1 || !strings.Contains(got[0], `"uri":"attachment:my%20notes.txt"`) {
		t.Errorf("got %v", got)
	}

	// Without embedded context, the file is fenced text.
	fenced := Attachment{Kind: KindText, Name: "doc.md", Text: "```go\nx\n```"}
	blocks := must(Blocks("", []Attachment{fenced}, acp.PromptCapabilities{}))
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "Attached file doc.md:\n\n````\n```go\nx\n```\n````" {
		t.Errorf("got %q", blocks[0].Text)
	}

	// Without image support, an image is an error.
	if _, err := Blocks("x", []Attachment{img}, acp.PromptCapabilities{EmbeddedContext: true}); !errors.Is(err, ErrNoImages) || !strings.Contains(err.Error(), "dot.png") {
		t.Errorf("got %v", err)
	}
	if _, err := Blocks("x", []Attachment{{Kind: "audio", Name: "a.wav"}}, all); err == nil {
		t.Error("unknown kind: no error")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

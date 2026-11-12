package loginenv

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	out := []byte("motd from .bashrc\n" + marker + "\nA=1\x00B=two\nlines\x00junk\x00=x\x00")
	got := parse(out)
	want := []string{"A=1", "B=two\nlines"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("parse = %q, want %q", got, want)
	}
	if parse([]byte("A=1\x00")) != nil {
		t.Fatal("output without the marker should be ignored")
	}
}

func TestBoundedBuffer(t *testing.T) {
	var b boundedBuffer
	big := strings.Repeat("x", maxOutput-1)
	if n, err := b.Write([]byte(big)); n != len(big) || err != nil {
		t.Fatal(n, err)
	}
	if n, err := b.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatal(n, err)
	}
	if b.buf.Len() != maxOutput || !strings.HasSuffix(b.buf.String(), "xa") {
		t.Fatalf("kept %d bytes", b.buf.Len())
	}
}

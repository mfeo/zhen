package ui

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestOSC52SequenceEncodesUTF8(t *testing.T) {
	seq := osc52Sequence("今天天氣很好")
	if !strings.HasPrefix(seq, "\x1b]52;c;") || !strings.HasSuffix(seq, "\x07") {
		t.Fatalf("malformed OSC 52 sequence: %q", seq)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]52;c;"), "\x07")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}
	if string(decoded) != "今天天氣很好" {
		t.Fatalf("decoded %q, want 今天天氣很好", decoded)
	}
}

func TestOSC52SequenceEmpty(t *testing.T) {
	if got := osc52Sequence(""); got != "\x1b]52;c;\x07" {
		t.Fatalf("got %q", got)
	}
}

func TestCopyToClipboardReturnsACmd(t *testing.T) {
	if copyToClipboard("hi") == nil {
		t.Fatal("copyToClipboard returned a nil Cmd")
	}
}

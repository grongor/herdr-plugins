package tui

import (
	"strconv"
	"strings"
	"testing"
)

// runes spells out anything that is not a plain character, so a stray Enter shows up in the failure.
func runes(keys []Key) string {
	var b strings.Builder
	for _, k := range keys {
		switch k.Kind {
		case KeyRune:
			b.WriteRune(k.Rune)
		case KeyEnter:
			b.WriteString("<enter>")
		case KeyTab:
			b.WriteString("<tab>")
		default:
			b.WriteString("<other>")
		}
	}

	return b.String()
}

// A branch name copied from a browser comes with a trailing newline, which without bracketed paste is Enter.
func TestPasteDoesNotSubmit(t *testing.T) {
	keys := (&Decoder{}).Keys([]byte(pasteStart + "feature/PROJ-123\n" + pasteEnd))

	if got := runes(keys); got != "feature/PROJ-123" {
		t.Fatalf("pasted %q, want the branch name and no enter", got)
	}
}

// A pasted tab would move to the next field and land the rest of the text there.
func TestPasteDropsControlCharacters(t *testing.T) {
	keys := (&Decoder{}).Keys([]byte(pasteStart + "a\tb\r\nc\x1bd" + pasteEnd))

	if got := runes(keys); got != "abcd" {
		t.Fatalf("pasted %q, want abcd", got)
	}
}

func TestTypedNewlineStillSubmits(t *testing.T) {
	keys := (&Decoder{}).Keys([]byte("ab\n"))

	if got := runes(keys); got != "ab<enter>" {
		t.Fatalf("typed %q, want ab<enter>", got)
	}
}

// Input is read a fixed number of bytes at a time, so a real paste arrives in pieces. The split runs at every size from
// two, because a chunk of exactly one ESC is the Escape key.
func TestPasteSpansChunks(t *testing.T) {
	body := strings.Repeat("a", 700)
	stream := pasteStart + body + "\n" + pasteEnd

	for _, size := range []int{2, 6, 7, 255, 256, 257, 512} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			var (
				dec  Decoder
				keys []Key
			)
			for i := 0; i < len(stream); i += size {
				keys = append(keys, dec.Keys([]byte(stream[i:min(i+size, len(stream))]))...)
			}

			if got := runes(keys); got != body {
				t.Fatalf("pasted %d characters (%.16q…), want %d and no enter", len(got), got, len(body))
			}
		})
	}
}

// The delimiter itself can be cut in half, which is why the search runs over what has been accumulated.
func TestPasteDelimiterSplitAcrossChunks(t *testing.T) {
	var dec Decoder

	keys := dec.Keys([]byte(pasteStart + "topic\x1b["))
	keys = append(keys, dec.Keys([]byte("201~x"))...)

	if got := runes(keys); got != "topicx" {
		t.Fatalf("pasted %q, want topicx", got)
	}
	if dec.pasting {
		t.Fatal("the paste was left open")
	}
}

// Decoding per chunk would corrupt any character a read boundary fell inside.
func TestPasteKeepsMultiByteRunes(t *testing.T) {
	var (
		dec  Decoder
		keys []Key
	)

	text := []byte("日本語")
	keys = append(keys, dec.Keys([]byte(pasteStart))...)
	// Split inside the first character.
	keys = append(keys, dec.Keys(text[:2])...)
	keys = append(keys, dec.Keys(text[2:])...)
	keys = append(keys, dec.Keys([]byte(pasteEnd))...)

	if got := runes(keys); got != "日本語" {
		t.Fatalf("pasted %q, want 日本語", got)
	}
}

func TestKeysAroundAPasteAreUnaffected(t *testing.T) {
	keys := (&Decoder{}).Keys([]byte("a" + pasteStart + "bc" + pasteEnd + "d\n"))

	if got := runes(keys); got != "abcd<enter>" {
		t.Fatalf("decoded %q, want abcd<enter>", got)
	}
}

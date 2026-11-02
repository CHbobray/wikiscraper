package jsonlines

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

type record struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWriteProducesOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	records := []record{{"a", 1}, {"b <&> c", 2}}
	if err := Write(&buf, records); err != nil {
		t.Fatal(err)
	}
	want := "{\"name\":\"a\",\"count\":1}\n{\"name\":\"b <&> c\",\"count\":2}\n"
	if got := buf.String(); got != want {
		t.Errorf("Write() =\n%s\nwant\n%s", got, want)
	}
}

func TestMultilineTextStaysOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []record{{Name: "line one\nline two"}}); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(buf.String(), "\n"); lines != 1 {
		t.Errorf("output has %d line feeds, want 1: %q", lines, buf.String())
	}
}

func TestRoundTripThroughFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jl")
	want := []record{{"robot", 3}, {"chatbot", 7}}
	if err := WriteFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile[record](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("round trip = %v, want %v", got, want)
	}
}

func TestReadAcceptsCRLFAndBlankLines(t *testing.T) {
	input := "{\"name\":\"a\",\"count\":1}\r\n\r\n{\"name\":\"b\",\"count\":2}\r\n"
	got, err := Read[record](strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "b" {
		t.Errorf("Read() = %v", got)
	}
}

func TestReadReportsBadLineNumber(t *testing.T) {
	input := "{\"name\":\"a\"}\nnot json\n"
	_, err := Read[record](strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("Read() error = %v, want an error naming line 2", err)
	}
}

func TestWriteFileFailsForMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "out.jl")
	if err := WriteFile(path, []record{{"a", 1}}); err == nil {
		t.Error("WriteFile into a missing directory should fail")
	}
}

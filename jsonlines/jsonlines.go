// Package jsonlines reads and writes newline-delimited JSON (JSON Lines,
// https://jsonlines.org). Each record is encoded as one compact JSON object
// followed by a single line feed. The package is generic and has no
// dependency on the scraper, so it can be reused in other projects.
package jsonlines

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// maxLineBytes bounds the size of a single JSON line when reading.
// Long Wikipedia articles can exceed bufio.Scanner's 64 KB default.
const maxLineBytes = 16 * 1024 * 1024

// Write encodes each record as one JSON object per line.
// HTML escaping is disabled so characters such as <, > and & stay readable.
func Write[T any](w io.Writer, records []T) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	for i, record := range records {
		// Encode appends the trailing '\n' that separates JSON Lines records.
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encoding record %d: %w", i, err)
		}
	}
	return nil
}

// WriteFile creates (or truncates) path and writes records to it.
func WriteFile[T any](path string, records []T) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	buffered := bufio.NewWriter(file)
	writeErr := Write(buffered, records)
	if writeErr == nil {
		writeErr = buffered.Flush()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// Read decodes every non-blank line of r into a T.
// It accepts both LF and CRLF line endings.
func Read[T any](r io.Reader) ([]T, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)

	var records []T
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if isBlank(line) {
			continue
		}
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading JSON lines: %w", err)
	}
	return records, nil
}

// ReadFile opens path and decodes it with Read.
func ReadFile[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return Read[T](file)
}

// isBlank reports whether a line holds only whitespace (including a stray '\r').
func isBlank(line []byte) bool {
	for _, b := range line {
		if b != ' ' && b != '\t' && b != '\r' {
			return false
		}
	}
	return true
}

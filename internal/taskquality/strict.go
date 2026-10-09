package taskquality

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// DecodeEvidenceJSON rejects lossy encoding, duplicate keys and excessive
// nesting before decoding bounded evidence into the supplied target.
func DecodeEvidenceJSON(raw []byte, target any) error { return strictJSON(raw, target) }

// Evidence and scored answers must not accept ambiguous, lossy JSON.
func strictJSON(raw []byte, target any) error {
	if len(raw) > 1<<20 || !utf8.Valid(raw) {
		return errors.New("invalid JSON evidence size or encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 64 {
			return errors.New("JSON evidence nesting limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delimiter != '{' && delimiter != '[' {
			return errors.New("unexpected JSON delimiter")
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delimiter == '{' {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate JSON evidence key")
				}
				seen[name] = true
			}
			if err := visit(depth + 1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON evidence")
	}
	return json.Unmarshal(raw, target)
}

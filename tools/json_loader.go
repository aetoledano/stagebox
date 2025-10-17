package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// LoadJSONFile opens `path` (or reads stdin if path == "-") and decodes JSON into v.
// Uses only the Go standard library.
func LoadJSONFile[T any](path string, v *T) error {
	var r io.ReadCloser
	if path == "-" {
		r = io.NopCloser(os.Stdin)
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		r = f
	}
	defer r.Close()

	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields() // strict: catches unknown keys

	if err := dec.Decode(v); err != nil {
		return err
	}
	// Ensure there isn't trailing junk after the JSON value
	// (optional but nice for strictness)
	if dec.More() {
		return fmt.Errorf("trailing data after JSON")
	}
	return nil
}

package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// LoadJSONFile opens `path` (or reads stdin if path == "-") and decodes JSON into v.
// Uses only the Go standard library.
func LoadJSONFile[T any](path string, v *T) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // strict: catches unknown keys

	if err := dec.Decode(v); err != nil {
		return err
	}
	// Ensure there isn't trailing junk after the JSON value
	// (optional but nice for strictness)
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("trailing data after JSON")
	}
	return nil
}

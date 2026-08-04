package exporters

// JSON writer — emits an array of objects, one per row. Object keys
// follow the Headers order (Go's json package preserves struct field
// order but maps don't, so we use json.Encoder + a manual buffered
// writer to keep header order stable).
//
// Why arrays-of-objects and not arrays-of-arrays: scripts consuming
// the export almost always want named fields ("the URL column" not
// "column 4"). The few extra bytes per row for repeated keys are
// dwarfed by gzip / network overhead in real-world use.

import (
	"encoding/json"
	"io"
	"strings"
)

func writeJSON(w io.Writer, headers []string, rows []ExportRow) error {
	// Hand-build the JSON so we can preserve column order across the
	// file. encoding/json sorts map keys alphabetically, which would
	// reorder our headers; building strings + json.Encoder for value
	// escaping gives us order + correctness in one pass.
	var b strings.Builder
	b.WriteString("[")
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("{")
		for j, h := range headers {
			if j > 0 {
				b.WriteString(",")
			}
			// Encode the key + value separately so escaping is
			// handled by encoding/json without us having to roll
			// our own.
			keyJSON, _ := json.Marshal(h)
			valJSON, _ := json.Marshal(r.Get(h))
			b.Write(keyJSON)
			b.WriteString(":")
			b.Write(valJSON)
		}
		b.WriteString("}")
	}
	b.WriteString("]\n")
	_, err := io.WriteString(w, b.String())
	return err
}

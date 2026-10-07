package exporters

// JSON writer — emits an array of objects, one per row. Object keys
// follow the Headers order (Go's json package preserves struct field
// order but maps don't, so we use json.Encoder + a manual buffered
// writer to keep header order stable).

import (
	"encoding/json"
	"io"
	"strings"
)

func writeJSON(w io.Writer, headers []string, rows []ExportRow) error {
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

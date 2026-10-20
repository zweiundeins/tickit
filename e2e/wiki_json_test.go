package e2e

import (
	"encoding/json"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/doc"
	"github.com/mbolli/tickit/internal/wiki"
)

type writerTx = writer.Tx

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func marshal(h wiki.Head) (string, error) {
	if h.Doc == nil {
		return "", nil
	}
	b, err := doc.MarshalDoc(h.Doc)
	return string(b), err
}

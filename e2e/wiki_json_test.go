package e2e

import (
	"encoding/json"

	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/wiki"
)

type writerTx = writer.Tx

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func marshal(h wiki.Head) (string, error) {
	if h.Doc == nil {
		return "", nil
	}
	b, err := prosemirror.MarshalDoc(h.Doc)
	return string(b), err
}

package ui

import (
	"os"
	"strings"
	"testing"
)

// PanelIcons are the icons prose.css draws in a document's panels, by
// kind; the stylesheet holds their masks as IconMask writes them.
var PanelIcons = map[string]string{"info": "info", "note": "edit", "warning": "warning", "success": "check", "error": "close"}

func TestProsePanelIcons(t *testing.T) {
	b, err := os.ReadFile("../web/static/css/prose.css")
	if err != nil {
		t.Fatal(err)
	}
	for kind, icon := range PanelIcons {
		want := `&[data-panel="` + kind + `"] {` + "\n\t\t\t\t--_icon: " + IconMask(icon) + ";"
		if !strings.Contains(string(b), want) {
			t.Errorf("prose.css lacks the %s panel's icon:\n%s", kind, want)
		}
	}
}

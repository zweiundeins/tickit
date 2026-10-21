package e2e

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
)

// pasteFile is a script that pastes a file of the bytes in base64 with
// the media type into the editor, as a browser does with a screenshot on
// the clipboard; drop drops it at the end of the document instead.
func pasteFile(b64, typ string, drop bool) string {
	ev := `new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})`
	if drop {
		ev = `(() => { const r = pm.lastElementChild.getBoundingClientRect(); return new DragEvent('drop', {dataTransfer: dt, bubbles: true, cancelable: true, clientX: r.left + 4, clientY: r.bottom - 4}) })()`
	}
	return `(() => {
		const bytes = Uint8Array.from(atob('` + b64 + `'), (c) => c.charCodeAt(0));
		const dt = new DataTransfer();
		dt.items.add(new File([bytes], 'shot', {type: '` + typ + `'}));
		const pm = document.querySelector('.pm-mount.ProseMirror, .pm-mount .ProseMirror');
		pm.dispatchEvent(` + ev + `);
		return true;
	})()`
}

func pngBase64(t *testing.T, w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.Set(1, 1, color.RGBA{200, 30, 30, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// FR-85.1: an image pasted into the editor in one tab is uploaded, goes
// into the page, and shows in another tab's reader as loaded; a dropped
// one too; a file the server refuses says why in the editor (MP-14).
func TestUploadAcrossTabs(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)
	editor, reader := b.NewPage(t, 1000, 700), b.NewPage(t, 1000, 700)
	editor.Navigate(t, edit)
	editorReady(t, editor)
	reader.Navigate(t, s.URL+"/pages/"+page)
	reader.Wait(t, `document.getElementById('page-body')`, 10*time.Second)
	defer func() {
		if t.Failed() {
			t.Logf("errors %v; logs %v", editor.Errors(), editor.Logs())
		}
	}()

	editor.Eval(t, pasteFile(pngBase64(t, 60, 40), "image/png", false), nil)
	editor.Wait(t, `document.querySelector('.pm-mount img[src^="/img/lib/up-"]')`, 15*time.Second)
	reader.Wait(t, `(() => { const i = document.querySelector('#page-body img[src^="/img/lib/up-"]'); return i && i.complete && i.naturalWidth === 60 })()`, 10*time.Second)

	editor.Eval(t, pasteFile(pngBase64(t, 30, 20), "image/png", true), nil)
	reader.Wait(t, `document.querySelectorAll('#page-body img[src^="/img/lib/up-"]').length === 2`, 15*time.Second)

	editor.Eval(t, pasteFile(base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n<script>alert(1)</script>")), "image/png", false), nil)
	editor.Wait(t, `document.getElementById('editor').shadowRoot.getElementById('notice-text').textContent === 'This file is not an image that can be read.'`, 10*time.Second)
	editor.Eval(t, pasteFile(base64.StdEncoding.EncodeToString([]byte("<svg/>")), "image/svg+xml", false), nil)
	editor.Wait(t, `document.getElementById('editor').shadowRoot.getElementById('notice-text').textContent.startsWith('Only PNG, JPEG and WebP')`, 5*time.Second)
	var n int
	editor.Eval(t, `document.querySelectorAll('.pm-mount img[src^="/img/lib/up-"], .pm-mount .sb-upload').length`, &n)
	if n != 2 {
		t.Errorf("the editor holds %d images and placeholders, want the 2 uploaded", n)
	}
	if logs := strings.Join(editor.Errors(), "\n"); strings.Contains(logs, "Content Security Policy") {
		t.Errorf("CSP: %s", logs)
	}
}

package web_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// photo is a JPEG with an Exif segment that holds a position.
func photo(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	app1 := append([]byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08\x00\x00"), "GPS 46.9480N 7.4474E"...)
	seg := append([]byte{0xff, 0xe1}, binary.BigEndian.AppendUint16(nil, uint16(len(app1)+2))...)
	return append(append(append([]byte{}, b.Bytes()[:2]...), append(seg, app1...)...), b.Bytes()[2:]...)
}

// uploadTo posts file as the tab's upload into the editor's subject.
func uploadTo(t *testing.T, c *http.Client, url, tab string, file []byte) (int, string) {
	t.Helper()
	return webtest.Upload(t, c, url, map[string]string{"tab": tab, "op": fmt.Sprintf("up%d", opCount.Add(1))}, file)
}

// uploaded uploads file and returns the stored image's id.
func uploaded(t *testing.T, c *http.Client, url, tab string, file []byte) string {
	t.Helper()
	code, body := uploadTo(t, c, url, tab, file)
	var r images.Result
	if code != http.StatusOK || json.Unmarshal([]byte(body), &r) != nil || !images.IsUpload(r.ID) {
		t.Fatalf("upload: %d %s", code, body)
	}
	return r.ID
}

// FR-85.1, NFR-15.2: a Member uploads a photo into a page; the app serves
// it re-encoded without its Exif under a generated name, with its media
// type, nosniff and a long cache; a commit puts it into the page, and
// another tab's reader shows it.
func TestUploadIntoPage(t *testing.T) {
	e := startWiki(t)
	c, reader := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	webtest.Get(t, reader, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	read := webtest.Listen(t, reader, e.URL+"/stream?tab=rtab&v=pagedoc:"+page, nil)
	webtest.Next(t, read, `id="page-body"`)

	id := uploaded(t, c, e.URL+"/cmd/pages/"+page+"/image", "tab1", photo(t))
	res, err := c.Get(e.URL + "/img/lib/" + id)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	h := res.Header
	if res.StatusCode != http.StatusOK || h.Get("Content-Type") != images.JPEG || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Cache-Control"), "immutable") || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("NFR-15.2: %d %v", res.StatusCode, h)
	}
	if bytes.Contains(body, []byte("Exif")) || bytes.Contains(body, []byte("GPS")) {
		t.Error("FR-85.1: the served image keeps its Exif")
	}
	if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
		t.Errorf("the served image does not decode: %v", err)
	}

	hd := head(t, e, page)
	if code, _ := act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": hd.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	commit := func(ref, image string) int {
		code, _ := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit", fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":%q,"client":"0123456789ab","steps":[`+
			`{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"image","attrs":{"imageId":%q,"src":"/img/lib/%s","alt":"A photo"}}]}}]}`, hd.Epoch, hd.Version, ref, image, image))
		return code
	}
	if code := commit("i1", id); code != http.StatusNoContent {
		t.Fatalf("a commit of the uploaded image: %d", code)
	}
	webtest.Next(t, read, `src="/img/lib/`+id+`"`)
	if code := commit("i2", images.NewID()); code != http.StatusUnprocessableEntity {
		t.Errorf("DM-06.3: a commit of an image nobody uploaded: %d", code)
	}
}

// FR-85.1: issue descriptions take uploads too (FR-15 shares the editor).
func TestUploadIntoDescription(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	code, body := act(t, e, c, "/cmd/projects/create", "tab1", map[string]any{"key": "IMG", "name": "Images"})
	if code != http.StatusOK {
		t.Fatalf("project: %d %s", code, body)
	}
	var project string
	e.DB.R.QueryRow(`SELECT id FROM projects WHERE key = 'IMG'`).Scan(&project)
	if code, body := act(t, e, c, "/cmd/issues/create", "tab1", map[string]any{"project": project, "type": "task", "summary": "With a picture"}); code >= 300 {
		t.Fatalf("issue: %d %s", code, body)
	}
	var issue string
	e.DB.R.QueryRow(`SELECT id FROM issues WHERE key = 'IMG-1'`).Scan(&issue)
	uploaded(t, c, e.URL+"/cmd/issues/"+issue+"/description/image", "tab1", photo(t))
}

// FR-85.1, NFR-15.2: what is not an image of the limits is refused, in
// words in a notice on the tab's stream and a bare status for the
// editor's code, and stores nothing; so is a request that is no upload.
func TestUploadRefusals(t *testing.T) {
	e := webtest.StartWith(t, webtest.Options{Store: wikiStore(), Configure: web.Unlimited})
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	url := e.URL + "/cmd/pages/" + page + "/image"
	stream := webtest.Listen(t, c, e.URL+"/stream?tab=tabR", nil)
	for name, want := range map[string]struct {
		file []byte
		code int
		msg  string
	}{
		"HTML":            {[]byte("<html><script>alert(1)</script></html>"), http.StatusUnprocessableEntity, images.MsgFormat},
		"SVG":             {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), http.StatusUnprocessableEntity, images.MsgFormat},
		"a PNG signature": {append([]byte("\x89PNG\r\n\x1a\n"), "<script>alert(1)</script>"...), http.StatusUnprocessableEntity, images.MsgBroken},
		"over 5 MB":       {append(photo(t), make([]byte, images.MaxBytes)...), http.StatusRequestEntityTooLarge, images.MsgTooLarge},
		"empty":           {[]byte{}, http.StatusUnprocessableEntity, images.MsgEmpty},
	} {
		code, body := uploadTo(t, c, url, "tabR", want.file)
		if code != want.code || strings.TrimSpace(body) != "" {
			t.Errorf("%s: %d %q, want %d and no words", name, code, body, want.code)
		}
		webtest.Next(t, stream, `"msg":"`+want.msg+`"`)
	}
	if code, _ := webtest.Upload(t, c, url, map[string]string{"tab": "tab1", "op": "nofile"}, nil); code != http.StatusBadRequest {
		t.Errorf("no file: %d", code)
	}
	if code, _ := webtest.Post(t, c, url, `{"tab":"tab1","op":"json"}`); code != http.StatusBadRequest {
		t.Errorf("JSON: %d", code)
	}
	if code, _ := webtest.Upload(t, c, url, map[string]string{"tab": "tab1"}, photo(t)); code != http.StatusBadRequest {
		t.Errorf("no op: %d", code)
	}
	if code, _ := uploadTo(t, c, e.URL+"/cmd/pages/0192d000-0000-7000-8000-000000000001/image", "tab1", photo(t)); code != http.StatusNotFound {
		t.Errorf("NFR-16: an unknown page: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM uploads`); n != 0 {
		t.Errorf("%d uploads stored", n)
	}
}

// DM-04.1: a session uploads 3 images at once, then one every 30 s.
func TestUploadRateLimit(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	stream := webtest.Listen(t, c, e.URL+"/stream?tab=tabL", nil)
	for range 3 {
		uploaded(t, c, e.URL+"/cmd/pages/"+page+"/image", "tabL", photo(t))
	}
	code, _ := uploadTo(t, c, e.URL+"/cmd/pages/"+page+"/image", "tabL", photo(t))
	if code != http.StatusTooManyRequests {
		t.Errorf("DM-04.2: the fourth upload: %d", code)
	}
	webtest.Next(t, stream, `"msg":"Too many changes at once`)
}

// NFR-15: an upload whose Tab-Id and Op-Id headers name its tab and op is
// checked before its body: one into a page that does not exist, and one
// from a tab without a stream, hear so though the body is not even
// multipart.
func TestUploadCheckedBeforeItsBody(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	post := func(page, tab string) (int, string) {
		req, _ := http.NewRequest("POST", e.URL+"/cmd/pages/"+page+"/image", strings.NewReader("no form"))
		for k, v := range map[string]string{"Content-Type": "multipart/form-data; boundary=x", "Datastar-Request": "true", "Sec-Fetch-Site": "same-origin", "Tab-Id": tab, "Op-Id": "early1"} {
			req.Header.Set(k, v)
		}
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, strings.TrimSpace(string(b))
	}
	stream := webtest.Listen(t, c, e.URL+"/stream?tab=tabE", nil)
	if code, _ := post("0192d000-0000-7000-8000-000000000001", "tabE"); code != http.StatusNotFound {
		t.Errorf("an upload into a page that does not exist: %d", code)
	}
	webtest.Next(t, stream, `"msg":"This was deleted or moved meanwhile."`)
	if code, _ := post(page, "no-stream"); code != http.StatusConflict {
		t.Errorf("an upload from a tab without a stream: %d", code)
	}
}

// FR-85.2, DM-07.1: a moderator hides an uploaded image: every open page
// that shows it says so, and it is no longer served; a delete removes its
// bytes for good.
func TestModerateUpload(t *testing.T) {
	e := startWiki(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo, c, reader := webtest.Client(t), webtest.Client(t), webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	webtest.Get(t, c, e.URL+"/")
	webtest.Get(t, reader, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	id := uploaded(t, c, e.URL+"/cmd/pages/"+page+"/image", "tab1", photo(t))
	hd := head(t, e, page)
	act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": hd.Version, "client": "0123456789ab"})
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit", fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"m1","client":"0123456789ab","steps":[`+
		`{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"image","attrs":{"imageId":%q,"alt":""}}]}}]}`, hd.Epoch, hd.Version, id)); code != http.StatusNoContent {
		t.Fatalf("commit: %d", code)
	}
	read := webtest.Listen(t, reader, e.URL+"/stream?tab=rtab&v=pagedoc:"+page, nil)
	webtest.Next(t, read, `src="/img/lib/`+id+`"`)

	if code, p := webtest.Get(t, mo, e.URL+"/moderation"); code != http.StatusOK || !strings.Contains(p, `data-id="`+id+`"`) || !strings.Contains(p, `class="tk-modlist__thumb" src="/img/lib/`+id+`"`) {
		t.Fatalf("DM-07.3: the moderation page does not list the upload: %d", code)
	}
	if code, b := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"h1","action":"hide","kind":"upload","id":"`+id+`","reason":"not here"}`); code != http.StatusNoContent {
		t.Fatalf("hide: %d %s", code, b)
	}
	if frame := webtest.Next(t, read, "Image hidden by a moderator"); strings.Contains(frame, `/img/lib/`+id+`"`) {
		t.Error("FR-85.2: the open page still shows the image")
	}
	if code, _ := webtest.Get(t, reader, e.URL+"/img/lib/"+id); code != http.StatusNotFound {
		t.Errorf("FR-85.2: the hidden image is served: %d", code)
	}
	if code, b := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"d1","action":"delete","kind":"upload","id":"`+id+`","reason":"illegal"}`); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, b)
	}
	if o := outcome(t, e, people.Person{}, "d1"); !o.OK {
		t.Fatalf("delete: %+v", o)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM uploads`); n != 0 {
		t.Error("DM-07.1: the deleted image's bytes stay")
	}
	if n := rowCount(t, e, `SELECT count(*) FROM moderation_log WHERE target_kind = 'upload'`); n != 2 {
		t.Errorf("DM-07.4: %d moderation log rows", n)
	}
	if _, p := webtest.Get(t, reader, e.URL+"/pages/"+page); strings.Contains(p, `src="/img/lib/`+id+`"`) {
		t.Error("the deleted image is still in the page")
	}
}

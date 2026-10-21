package images_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"slices"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

var (
	guest     = people.Person{ID: "g-aaaaaaaaaaaaaaaaaaaa", Kind: people.Guest, Name: "A guest"}
	other     = people.Person{ID: "g-bbbbbbbbbbbbbbbbbbbb", Kind: people.Guest, Name: "Another"}
	moderator = people.Person{ID: "a-0192d000-0000-7000-8000-000000000009", Kind: people.Account, Name: "Mo", Role: people.Moderator}
)

func by(p people.Person) tabs.By { return tabs.By{Actor: p, Op: "op", At: time.Now()} }

func open(tx *writer.Tx) error { return nil }

// tiny is an image Process made of a small PNG.
func tiny(t *testing.T) images.Image {
	var b bytes.Buffer
	png.Encode(&b, image.NewGray(image.Rect(0, 0, 3, 2)))
	im, err := images.Process(context.Background(), b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func upload(p people.Person, im images.Image, gate func(*writer.Tx) error) images.Upload {
	return images.Upload{By: by(p), ID: images.NewID(), Subject: images.OnPage, On: "0192d000-0000-7000-8000-000000000001", Image: im, Gate: gate}
}

func result(t *testing.T, e *dbtest.Env, cmd images.Upload) (images.Result, error) {
	t.Helper()
	var raw []byte
	err := e.W.Do(context.Background(), writer.Func(func(tx *writer.Tx) error {
		var err error
		raw, err = cmd.ApplyResult(tx)
		return err
	}))
	var r images.Result
	if err == nil {
		err = json.Unmarshal(raw, &r)
	}
	return r, err
}

// FR-85.1: an upload stores the image under its generated id, which
// /img/lib/ serves with its media type, and touches images.
func TestUploadStores(t *testing.T) {
	e := dbtest.Open(t)
	im := tiny(t)
	cmd := upload(guest, im, open)
	r, err := result(t, e, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != cmd.ID || r.Width != 3 || r.Height != 2 {
		t.Fatalf("%+v", r)
	}
	if !slices.Contains(e.Touched(), images.Topic) {
		t.Error("an upload touches images")
	}
	ctx := context.Background()
	ok, err := images.Exists(ctx, e.DB.R, r.ID)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	typ, data, err := images.Get(ctx, e.DB.R, r.ID)
	if err != nil || typ != images.PNG || !bytes.Equal(data, im.Data) {
		t.Fatalf("%s %v", typ, err)
	}
	if _, _, err := images.Get(ctx, e.DB.R, images.NewID()); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("an unknown id: %v", err)
	}
	if ok, _ := images.Exists(ctx, e.DB.R, "architecture-overview"); ok {
		t.Error("a library id is no upload")
	}
}

// FR-85.1: the gate decides inside the writer; its refusal stores nothing.
func TestUploadGate(t *testing.T) {
	e := dbtest.Open(t)
	no := &access.Denial{Refusal: writer.Refusal{Msg: "You may not."}}
	if _, err := result(t, e, upload(guest, tiny(t), func(*writer.Tx) error { return no })); !errors.Is(err, no) {
		t.Fatalf("%v", err)
	}
	var n int
	e.DB.R.QueryRow(`SELECT count(*) FROM uploads`).Scan(&n)
	if n != 0 {
		t.Errorf("%d stored", n)
	}
}

// DM-04: a person uploads PerPerson images between two resets, and the
// next is refused in words; another person still may.
func TestUploadCaps(t *testing.T) {
	e := dbtest.Open(t)
	im := tiny(t)
	for i := range images.PerPerson {
		if _, err := result(t, e, upload(guest, im, open)); err != nil {
			t.Fatalf("upload %d: %v", i+1, err)
		}
	}
	var r *writer.Refusal
	if _, err := result(t, e, upload(guest, im, open)); !errors.As(err, &r) || r.Msg == "" {
		t.Fatalf("DM-04: upload %d: %v", images.PerPerson+1, err)
	}
	if _, err := result(t, e, upload(other, im, open)); err != nil {
		t.Fatal(err)
	}
	// The bytes of all uploads together.
	big := im
	big.Data = bytes.Repeat([]byte{1}, images.MaxBytes)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE uploads SET bytes = ?`, images.SiteBytes/images.PerPerson)
		return err
	}))
	third := people.Person{ID: "g-cccccccccccccccccccc", Kind: people.Guest}
	if _, err := result(t, e, upload(third, big, open)); !errors.As(err, &r) || r.Msg != "The demo is full until the next reset" {
		t.Fatalf("the site's bytes: %v", err)
	}
}

// DM-07.1: a moderator deletes an uploaded image; nobody else does.
func TestDelete(t *testing.T) {
	e := dbtest.Open(t)
	r, err := result(t, e, upload(guest, tiny(t), open))
	if err != nil {
		t.Fatal(err)
	}
	var denial *access.Denial
	if err := e.W.Do(context.Background(), images.Delete{By: by(guest), ID: r.ID}); !errors.As(err, &denial) {
		t.Fatalf("a guest deletes: %v", err)
	}
	e.Do(t, images.Delete{By: by(moderator), ID: r.ID})
	if ok, _ := images.Exists(context.Background(), e.DB.R, r.ID); ok {
		t.Error("still there")
	}
	if err := e.W.Do(context.Background(), images.Delete{By: by(moderator), ID: r.ID}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("twice: %v", err)
	}
}

// Validate refuses what Process did not make.
func TestUploadValidate(t *testing.T) {
	im := tiny(t)
	for name, c := range map[string]images.Upload{
		"no actor":   {ID: images.NewID(), Subject: images.OnPage, On: "x", Image: im, Gate: open},
		"no id":      {By: by(guest), Subject: images.OnPage, On: "x", Image: im, Gate: open},
		"a page id":  {By: by(guest), ID: "architecture-overview", Subject: images.OnPage, On: "x", Image: im, Gate: open},
		"no subject": {By: by(guest), ID: images.NewID(), Subject: "comment", On: "x", Image: im, Gate: open},
		"no gate":    {By: by(guest), ID: images.NewID(), Subject: images.OnPage, On: "x", Image: im},
		"a GIF":      {By: by(guest), ID: images.NewID(), Subject: images.OnPage, On: "x", Image: images.Image{Type: "image/gif", Data: []byte("GIF89a")}, Gate: open},
	} {
		if c.Validate() == nil {
			t.Errorf("%s: valid", name)
		}
	}
}

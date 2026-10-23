package web

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/collab"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// Image uploads (FR-85, NFR-15). sb-rich-editor posts a file to its upload
// attribute, the editor's data-cmd and "/image", as a multipart body with
// the tab, the op id and the file, and Datastar's header for edge.CSRF.
// The handler checks the limits and the right, converts the file outside
// the writer (images.Process) and stores it through tabs.Act. It answers
// the stored image's {id, width, height} as JSON, or why not in words: 413
// for a file over 5 MB, 422 for a file it does not take or a refusal, 403
// without the right, 503 while another image converts.
//
//	POST /cmd/pages/{id}/image                 into a page's document
//	POST /cmd/issues/{id}/description/image    into an issue's description
//	GET  /img/lib/{id}                         an uploaded image, as the library's (routes_wiki.go)
func (s *Server) routesImages() {
	s.post("POST /cmd/pages/{id}/image", s.uploadImage(images.OnPage))
	s.post("POST /cmd/issues/{id}/description/image", s.uploadImage(images.OnIssue))
}

// convertWait is how long an upload waits for another one's conversion.
const convertWait = 20 * time.Second

// worded puts words into the bare statuses an upload answers (a rate
// limit, a tab without a stream, a failed write): sb-rich-editor sends it
// with a plain fetch, which reads the body, and has no op tracking.
type worded struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (w *worded) WriteHeader(code int) {
	if code < 400 || w.wrote {
		w.wrote = true
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.code = code
}

func (w *worded) Write(b []byte) (int, error) {
	if !w.wrote && w.code != 0 {
		w.ResponseWriter.WriteHeader(w.code)
	}
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// finish answers a status the handler left bare in words.
func (w *worded) finish() {
	if w.wrote || w.code == 0 {
		return
	}
	msg := map[int]string{
		http.StatusTooManyRequests:     tooMany,
		http.StatusConflict:            "This tab is not connected. Reload the page.",
		http.StatusForbidden:           "You cannot add images here.",
		http.StatusNotFound:            "This was deleted or moved meanwhile.",
		http.StatusServiceUnavailable:  restarting,
		http.StatusInternalServerError: notSaved,
	}[w.code]
	w.wrote = true
	http.Error(w.ResponseWriter, msg, w.code)
}

func (s *Server) uploadImage(subject string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		w := &worded{ResponseWriter: rw}
		defer w.finish()
		if !s.allow(w, r, classUpload) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, classUpload.body)
		p, file, err := readUpload(r)
		var tooLarge *http.MaxBytesError
		switch {
		case errors.Is(err, errFileTooLarge), errors.As(err, &tooLarge):
			http.Error(w, images.MsgTooLarge, http.StatusRequestEntityTooLarge)
			return
		case err != nil:
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		a, ok := s.actionOf(w, r, &p)
		if !ok {
			return
		}
		ctx, id := r.Context(), r.PathValue("id")
		// The right first, from a read: converting costs a second or two.
		allowed, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (bool, error) {
			if subject == images.OnPage {
				p, sp, err := readable(ctx, tx, a.By.Actor, id)
				return err == nil && p.State == wiki.Current && canIn(ctx, tx, a.By.Actor, access.EditIssues, sp.ID), err
			}
			_, ok, err := editableIssue(ctx, tx, a.By.Actor, id)
			return ok, err
		})
		switch {
		case err != nil:
			s.failOp(w, r, a, err)
			return
		case !allowed:
			http.Error(w, "You cannot add images here.", http.StatusForbidden)
			return
		}
		if !s.connected(w, r, a.By.Actor, a.Tab) || s.paused(w, r, a.By.Actor) {
			return
		}
		wait, cancel := context.WithTimeout(ctx, convertWait)
		img, err := images.Process(wait, file)
		cancel()
		var ref *writer.Refusal
		switch {
		case errors.As(err, &ref) && ref.Msg == images.MsgBusy:
			http.Error(w, ref.Msg, http.StatusServiceUnavailable)
			return
		case errors.As(err, &ref):
			http.Error(w, ref.Msg, http.StatusUnprocessableEntity)
			return
		case err != nil:
			s.fail(w, r, err)
			return
		}
		res, ok := s.run(w, r, a, images.Upload{By: a.By, ID: images.NewID(), Subject: subject, On: id, Image: img, Gate: uploadGate(subject, a.By, id)})
		if !ok {
			return
		}
		if !res.OK {
			msg := "This image was not stored."
			if st, err := tabs.Get(ctx, s.db.R, a.Session, a.Tab); err == nil {
				if o, ok := st.Outcome(a.By.Op); ok && o.Msg != "" {
					msg = o.Msg
				}
			}
			status := http.StatusUnprocessableEntity
			if res.Denied {
				status = http.StatusForbidden
			}
			http.Error(w, msg, status)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "no-store")
		w.Write(res.Value)
	}
}

// uploadGate checks in the writer that by may change the subject's
// document now, as its commits do.
func uploadGate(subject string, by tabs.By, id string) func(tx *writer.Tx) error {
	return func(tx *writer.Tx) error {
		var err error
		if subject == images.OnPage {
			err = wiki.DocEditable(tx, by, id)
		} else {
			err = tracker.DescriptionEditable(tx, by, id)
		}
		if errors.Is(err, collab.ErrGone) {
			return &writer.Refusal{Msg: "This " + map[string]string{images.OnPage: "page", images.OnIssue: "issue"}[subject] + " was removed."}
		}
		return err
	}
}

var errFileTooLarge = errors.New("web: the file is too large")

// readUpload reads an upload's multipart body in memory: the fields tab
// and op, and the file, of at most images.MaxBytes. Other parts are
// skipped; the body as a whole is bounded by the caller.
func readUpload(r *http.Request) (Payload, []byte, error) {
	var p Payload
	mr, err := r.MultipartReader()
	if err != nil {
		return p, nil, err
	}
	var file []byte
	for range 32 {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			if file == nil {
				return p, nil, errors.New("web: an upload without a file")
			}
			return p, file, nil
		}
		if err != nil {
			return p, nil, err
		}
		switch part.FormName() {
		case "tab":
			p.Tab, err = small(part)
		case "op":
			p.Op, err = small(part)
		case "file":
			if file != nil {
				return p, nil, errors.New("web: two files in an upload")
			}
			file, err = io.ReadAll(io.LimitReader(part, images.MaxBytes+1))
			if err == nil && len(file) > images.MaxBytes {
				err = errFileTooLarge
			}
		}
		if err != nil {
			return p, nil, err
		}
	}
	return p, nil, errors.New("web: too many parts in an upload")
}

// small reads a field of at most 64 bytes, as tab and op ids are.
func small(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, 65))
	if err != nil {
		return "", err
	}
	if len(b) > 64 {
		return "", errors.New("web: a field too long")
	}
	return string(b), nil
}

// uploadedImage serves an uploaded image (FR-85, NFR-15.2) with the media
// type Process gave it, never one a browser guesses: ids never name other
// bytes, so it caches for a year, privately, since a moderator may hide
// it. A hidden or deleted one answers 404 (DM-07.1).
func (s *Server) uploadedImage(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	hidden, err := wiki.ImageHidden(ctx, s.db.R, id)
	if err != nil || hidden {
		s.notFound(w, r)
		return
	}
	typ, data, err := images.Get(ctx, s.db.R, id)
	if err != nil {
		s.notFound(w, r)
		return
	}
	ext := ".png"
	if typ == images.JPEG {
		ext = ".jpg"
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Disposition", `inline; filename="`+id+ext+`"`)
	w.Write(data)
}

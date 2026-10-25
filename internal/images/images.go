// Package images holds the images people upload into pages and issue
// descriptions (FR-85, NFR-15). Process reads what a browser sends: a PNG,
// JPEG or WebP file of at most MaxBytes and MaxSide by MaxSide pixels,
// checked by its header before it is decoded, decoded with the decoder of
// the format its bytes say, turned upright by a JPEG's orientation, and
// encoded again as PNG or JPEG, which keeps none of the file's metadata or
// trailing bytes. Upload stores the result under a generated id (NewID) in
// the database, within the caps of DM-04; documents name it as they name an
// image of the library, and /img/lib/<id> serves it. Uploads are relational
// rows, without events: a reset empties them with the rest of the demo.
package images

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strings"

	"github.com/zweiundeins/lith/writer"
	"golang.org/x/image/webp"
)

// The limits of an upload (FR-85.1, NFR-15.2).
const (
	MaxBytes = 5 << 20 // the file as sent, and the image as stored
	MaxSide  = 4096    // pixels, either way
)

// Media types of what Process stores.
const (
	PNG  = "image/png"
	JPEG = "image/jpeg"
)

// JPEGQuality is the quality photos are encoded at again.
const JPEGQuality = 85

// Image is an image as Process made it, ready to store.
type Image struct {
	Type          string // PNG or JPEG
	Width, Height int
	Data          []byte
}

var idRe = regexp.MustCompile(`^up-[a-z2-7]{26}$`)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewID is an uploaded image's id: "up-" and 128 random bits in base32.
// It matches the documents' image ids (internal/docnodes) and no library id.
func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "up-" + strings.ToLower(b32.EncodeToString(b))
}

// IsUpload reports whether id has the form of an uploaded image's id.
func IsUpload(id string) bool { return idRe.MatchString(id) }

// Refusals of a file, in words for the person who sent it.
const (
	MsgEmpty    = "Choose an image file to upload."
	MsgTooLarge = "An image has at most 5 MB."
	MsgFormat   = "Only PNG, JPEG and WebP images can be uploaded."
	MsgBroken   = "This file is not an image that can be read."
	MsgTooBig   = "An image has at most 4,096 by 4,096 pixels."
	MsgTooHeavy = "This image is over 5 MB once converted. Upload a smaller one."
	MsgBusy     = "Another image is being converted. Try again in a moment."
)

func refuse(msg string) error { return &writer.Refusal{Msg: msg} }

type format int

const (
	unknown format = iota
	fmtPNG
	fmtJPEG
	fmtWebP
)

// sniff names a file's format by its first bytes, never by what the
// browser says it is.
func sniff(b []byte) format {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return fmtPNG
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return fmtJPEG
	case len(b) >= 16 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return fmtWebP
	}
	return unknown
}

type codec struct {
	decode func(io.Reader) (image.Image, error)
	config func(io.Reader) (image.Config, error)
}

var codecs = map[format]codec{
	fmtPNG:  {png.Decode, png.DecodeConfig},
	fmtJPEG: {jpeg.Decode, jpeg.DecodeConfig},
	fmtWebP: {webp.Decode, webp.DecodeConfig},
}

// converting lets one image at a time be decoded and encoded: a 4,096 by
// 4,096 one takes up to 250 MB while it is, and the host has 448 MiB.
var converting = make(chan struct{}, 1)

// Process decodes a file as a browser sends it and encodes it again
// (FR-85.1). A file it does not take is a *writer.Refusal with the reason
// in words; ctx bounds the wait for another conversion to end.
func Process(ctx context.Context, b []byte) (Image, error) {
	switch {
	case len(b) == 0:
		return Image{}, refuse(MsgEmpty)
	case len(b) > MaxBytes:
		return Image{}, refuse(MsgTooLarge)
	}
	f := sniff(b)
	c, ok := codecs[f]
	if !ok {
		return Image{}, refuse(MsgFormat)
	}
	// The header first: a small file may claim a huge image (NFR-15.2).
	cfg, err := safely(func() (image.Config, error) { return c.config(bytes.NewReader(b)) })
	if err != nil {
		return Image{}, refuse(MsgBroken)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxSide || cfg.Height > MaxSide {
		return Image{}, refuse(MsgTooBig)
	}
	select {
	case converting <- struct{}{}:
		defer func() { <-converting }()
	case <-ctx.Done():
		return Image{}, refuse(MsgBusy)
	}
	img, err := safely(func() (image.Image, error) { return c.decode(bytes.NewReader(b)) })
	if err != nil {
		return Image{}, refuse(MsgBroken)
	}
	if bd := img.Bounds(); bd.Dx() != cfg.Width || bd.Dy() != cfg.Height {
		return Image{}, refuse(MsgBroken)
	}
	if f == fmtJPEG {
		img = upright(img, orientation(b))
	}
	return encode(img, f == fmtPNG || (f == fmtWebP && webpLossless(b)))
}

// webpLossless reports whether a WebP file holds a lossless image (VP8L),
// by its chunks.
func webpLossless(b []byte) bool {
	for off := 12; off+8 <= len(b); {
		switch string(b[off : off+4]) {
		case "VP8L":
			return true
		case "VP8 ":
			return false
		}
		n := int(binary.LittleEndian.Uint32(b[off+4:]))
		if n < 0 || n > len(b) {
			return false
		}
		off += 8 + n + n&1
	}
	return false
}

// safely runs a decoder and turns a panic into an error: the decoders
// read bytes from anyone.
func safely[T any](f func() (T, error)) (v T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("images: the decoder failed: %v", r)
		}
	}()
	return f()
}

// encode writes img as PNG when it came lossless or has transparency, and
// as JPEG otherwise; a PNG over MaxBytes of an opaque image becomes a JPEG.
func encode(img image.Image, lossless bool) (Image, error) {
	opaque := isOpaque(img)
	out := Image{Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	var buf bytes.Buffer
	if lossless || !opaque {
		if err := png.Encode(&buf, img); err != nil {
			return Image{}, err
		}
		if buf.Len() <= MaxBytes {
			out.Type, out.Data = PNG, buf.Bytes()
			return out, nil
		}
		if !opaque {
			return Image{}, refuse(MsgTooHeavy)
		}
		buf.Reset()
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return Image{}, err
	}
	if buf.Len() > MaxBytes {
		return Image{}, refuse(MsgTooHeavy)
	}
	out.Type, out.Data = JPEG, buf.Bytes()
	return out, nil
}

func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// upright turns a JPEG as its Exif orientation (1 to 8) says, since the
// orientation goes with the rest of the metadata.
func upright(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	src := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			si, di := src.PixOffset(sx, sy), dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

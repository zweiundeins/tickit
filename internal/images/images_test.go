package images

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/docnodes"
)

// picture is a w by h image with a red top-left corner, a green top-right
// one, a blue bottom-left one and the rest grey, in blocks of 8 pixels
// so that JPEG keeps the colours.
func picture(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{128, 128, 128, 255}
			switch {
			case x < 8 && y < 8:
				c = color.RGBA{255, 0, 0, 255}
			case x >= w-8 && y < 8:
				c = color.RGBA{0, 255, 0, 255}
			case x < 8 && y >= h-8:
				c = color.RGBA{0, 0, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func pngOf(t testing.TB, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegOf(t testing.TB, img image.Image) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// chunk is a PNG chunk with its CRC.
func chunk(typ string, data []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	b = append(b, typ...)
	b = append(b, data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

// withPNGText puts a tEXt chunk before IEND.
func withPNGText(b []byte, text string) []byte {
	end := len(b) - 12
	out := append([]byte{}, b[:end]...)
	out = append(out, chunk("tEXt", []byte("Comment\x00"+text))...)
	return append(out, b[end:]...)
}

// withExif puts an APP1 Exif segment with an orientation and a secret,
// and a comment, after a JPEG's SOI.
func withExif(b []byte, orient int, secret string) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, uint16(orient))
	tiff = append(tiff, 0, 0)
	tiff = binary.BigEndian.AppendUint32(tiff, 0)
	tiff = append(tiff, secret...)
	app1 := append([]byte("Exif\x00\x00"), tiff...)
	seg := append([]byte{0xff, 0xe1}, binary.BigEndian.AppendUint16(nil, uint16(len(app1)+2))...)
	seg = append(seg, app1...)
	com := []byte("comment " + secret)
	seg = append(seg, 0xff, 0xfe)
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(com)+2))
	seg = append(seg, com...)
	out := append([]byte{}, b[:2]...)
	out = append(out, seg...)
	return append(out, b[2:]...)
}

func process(t testing.TB, b []byte) (Image, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return Process(ctx, b)
}

// stored checks what Process made: an image of the type it says, of the
// size it says, that decodes again.
func stored(t *testing.T, im Image, typ string, w, h int) image.Image {
	t.Helper()
	if im.Type != typ || im.Width != w || im.Height != h {
		t.Fatalf("got %s %dx%d, want %s %dx%d", im.Type, im.Width, im.Height, typ, w, h)
	}
	if got := http.DetectContentType(im.Data); got != typ {
		t.Fatalf("the bytes sniff as %s, not %s", got, typ)
	}
	img, _, err := image.Decode(bytes.NewReader(im.Data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != w || b.Dy() != h {
		t.Fatalf("decodes as %v", b)
	}
	return img
}

// FR-85.1: a PNG comes back as a PNG without its text chunks or the bytes
// after its end, with the same pixels.
func TestPNGStripped(t *testing.T) {
	src := picture(40, 24)
	in := withPNGText(pngOf(t, src), "SECRET-AUTHOR")
	in = append(in, "<script>alert(1)</script>PK\x03\x04"...) // a polyglot's tail
	im, err := process(t, in)
	if err != nil {
		t.Fatal(err)
	}
	img := stored(t, im, PNG, 40, 24)
	for _, s := range []string{"tEXt", "SECRET", "<script", "PK\x03\x04"} {
		if bytes.Contains(im.Data, []byte(s)) {
			t.Errorf("FR-85.1: %q is still in the image", s)
		}
	}
	if r, g, b, _ := img.At(0, 0).RGBA(); r>>8 != 255 || g != 0 || b != 0 {
		t.Errorf("the pixels changed: %v", img.At(0, 0))
	}
}

// FR-85.1: a JPEG's Exif (with a GPS position, say) and comments go; its
// orientation turns the image upright first.
func TestJPEGExifStripped(t *testing.T) {
	src := picture(48, 32)
	for o, corner := range map[int]image.Point{1: {0, 0}, 3: {47, 31}, 6: {31, 0}, 8: {0, 47}} {
		in := withExif(jpegOf(t, src), o, "SECRET-GPS-46.95N")
		if got := orientation(in); got != o {
			t.Fatalf("orientation %d read as %d", o, got)
		}
		im, err := process(t, in)
		if err != nil {
			t.Fatal(err)
		}
		w, h := 48, 32
		if o >= 5 {
			w, h = h, w
		}
		img := stored(t, im, JPEG, w, h)
		for _, s := range []string{"Exif", "SECRET", "comment"} {
			if bytes.Contains(im.Data, []byte(s)) {
				t.Errorf("FR-85.1: orientation %d: %q is still in the image", o, s)
			}
		}
		if orientation(im.Data) != 0 {
			t.Errorf("orientation %d: the image still has an orientation", o)
		}
		// The red corner went where the orientation puts the top-left.
		r, g, b, _ := img.At(corner.X, corner.Y).RGBA()
		if r>>8 < 200 || g>>8 > 60 || b>>8 > 60 {
			t.Errorf("orientation %d: the red corner is not at %v: %d %d %d", o, corner, r>>8, g>>8, b>>8)
		}
	}
}

// Every orientation moves the corners as Exif says.
func TestUpright(t *testing.T) {
	src := picture(16, 24)
	red, green := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}
	// Where the top-left and the top-right corner go, at 0 or 1 of the
	// width and the height.
	want := map[int][2]image.Point{
		1: {{0, 0}, {1, 0}}, 2: {{1, 0}, {0, 0}}, 3: {{1, 1}, {0, 1}}, 4: {{0, 1}, {1, 1}},
		5: {{0, 0}, {0, 1}}, 6: {{1, 0}, {1, 1}}, 7: {{1, 1}, {1, 0}}, 8: {{0, 1}, {0, 0}},
	}
	for o, w := range want {
		img := upright(src, o)
		b := img.Bounds()
		at := func(p image.Point) color.Color { return img.At(p.X*(b.Dx()-1), p.Y*(b.Dy()-1)) }
		if at(w[0]) != color.Color(red) || at(w[1]) != color.Color(green) {
			t.Errorf("orientation %d: %v %v", o, at(w[0]), at(w[1]))
		}
		if (o >= 5) != (b.Dx() == 24) {
			t.Errorf("orientation %d: %v", o, b)
		}
	}
}

// FR-85.1: WebP decodes; a lossy one becomes a JPEG, a lossless one or
// one with transparency a PNG.
func TestWebP(t *testing.T) {
	for file, typ := range map[string]string{
		"blue-purple-pink.lossy.webp":       JPEG,
		"gopher-doc.1bpp.lossless.webp":     PNG,
		"yellow_rose.lossy-with-alpha.webp": PNG,
	} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		im, err := process(t, b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		cfg, err := webpConfig(b)
		if err != nil {
			t.Fatal(err)
		}
		stored(t, im, typ, cfg.Width, cfg.Height)
	}
}

func webpConfig(b []byte) (image.Config, error) { return codecs[fmtWebP].config(bytes.NewReader(b)) }

// refusal is the message of a refused file.
func refusal(t *testing.T, b []byte) string {
	t.Helper()
	_, err := process(t, b)
	var r *writer.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("not refused: %v", err)
	}
	return r.Msg
}

// FR-85.1, NFR-15.2: what is not a PNG, JPEG or WebP image of at most 5 MB
// and 4,096 by 4,096 pixels is refused in words, whatever it claims.
func TestRefused(t *testing.T) {
	small := pngOf(t, picture(16, 16))
	jpg := jpegOf(t, picture(16, 16))
	gopher, _ := os.ReadFile("testdata/gopher-doc.1bpp.lossless.webp")
	cases := []struct {
		name string
		b    []byte
		want string
	}{
		{"empty", nil, MsgEmpty},
		{"over 5 MB", append(append([]byte{}, small...), make([]byte, MaxBytes)...), MsgTooLarge},
		{"HTML", []byte("<!doctype html><script>alert(1)</script>"), MsgFormat},
		{"SVG", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), MsgFormat},
		{"GIF", []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00"), MsgFormat},
		{"PDF", []byte("%PDF-1.7\n"), MsgFormat},
		{"HTML behind a PNG signature", append([]byte("\x89PNG\r\n\x1a\n"), "<html><script>alert(1)</script>"...), MsgBroken},
		{"a PNG cut short", small[:len(small)/2], MsgBroken},
		{"a JPEG cut short", jpg[:len(jpg)/3], MsgBroken},
		{"a PNG 8,192 pixels wide", pngHeader(8192, 16), MsgTooBig},
		{"a PNG 5,000 by 5,000 pixels", pngHeader(5000, 5000), MsgTooBig},
		{"a PNG of no pixels", pngHeader(0, 16), MsgBroken},
		{"a JPEG 5,000 pixels high", jpegSized(t, jpg, 16, 5000), MsgTooBig},
		{"a WebP 5,000 pixels wide", webpSized(gopher, 5000, 10), MsgTooBig},
		{"RIFF that is not WebP", []byte("RIFF\x10\x00\x00\x00WAVEfmt \x00\x00\x00\x00"), MsgFormat},
	}
	for _, c := range cases {
		if got := refusal(t, c.b); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// NFR-15.2: a decompression bomb is refused by its header, before anything
// is decoded: a PNG of 4,097 by 4,097 pixels of one colour in a few KB.
func TestBombRefusedByHeader(t *testing.T) {
	b := pngHeader(4097, 4097)
	start := time.Now()
	if got := refusal(t, b); got != MsgTooBig {
		t.Fatalf("%q", got)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("the refusal took %v: it decoded", d)
	}
}

// pngHeader is a PNG whose IHDR says w by h, with a little image data.
func pngHeader(w, h int) []byte {
	ihdr := binary.BigEndian.AppendUint32(nil, uint32(w))
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h))
	ihdr = append(ihdr, 8, 0, 0, 0, 0) // 8-bit grey
	b := []byte("\x89PNG\r\n\x1a\n")
	b = append(b, chunk("IHDR", ihdr)...)
	b = append(b, chunk("IDAT", []byte{0x78, 0x9c, 0x63, 0, 0, 0, 1, 0, 1})...)
	return append(b, chunk("IEND", nil)...)
}

// jpegSized patches a JPEG's frame header to say w by h.
func jpegSized(t *testing.T, b []byte, w, h int) []byte {
	out := append([]byte{}, b...)
	i := bytes.Index(out, []byte{0xff, 0xc0})
	if i < 0 {
		t.Fatal("no SOF0")
	}
	binary.BigEndian.PutUint16(out[i+5:], uint16(h))
	binary.BigEndian.PutUint16(out[i+7:], uint16(w))
	return out
}

// webpSized patches a lossless WebP's header to say w by h.
func webpSized(b []byte, w, h int) []byte {
	out := append([]byte{}, b...)
	i := bytes.Index(out, []byte("VP8L")) + 8 // after the chunk header
	v := binary.LittleEndian.Uint32(out[i+1:])
	v = v&^(1<<28-1) | uint32(w-1) | uint32(h-1)<<14
	binary.LittleEndian.PutUint32(out[i+1:], v)
	return out
}

// An opaque PNG too large as PNG goes out as a JPEG; one with
// transparency is refused.
func TestHeavyPNG(t *testing.T) {
	noise := func(alpha bool) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, 1400, 1400))
		x := uint32(1)
		for i := range img.Pix {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			img.Pix[i] = byte(x)
			if i%4 == 3 && !alpha {
				img.Pix[i] = 255
			}
		}
		return img
	}
	b := pngOf(t, noise(false))
	if len(b) <= MaxBytes {
		t.Fatalf("the noise is only %d bytes as PNG", len(b))
	}
	im, err := encode(noise(false), true)
	if err != nil || im.Type != JPEG {
		t.Fatalf("%v %s", err, im.Type)
	}
	if _, err := encode(noise(true), true); err == nil || err.Error() != MsgTooHeavy {
		t.Fatalf("%v", err)
	}
}

// One conversion at a time: another waits, and gives up with the context.
func TestBusy(t *testing.T) {
	converting <- struct{}{}
	defer func() { <-converting }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := Process(ctx, pngOf(t, picture(16, 16)))
	if err == nil || err.Error() != MsgBusy {
		t.Fatalf("%v", err)
	}
}

// An uploaded image's id is a document's image id, and no library one.
func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewID()
		if !IsUpload(id) || seen[id] {
			t.Fatalf("%q", id)
		}
		seen[id] = true
		d, err := docnodes.ParseDoc([]byte(`{"type":"doc","content":[{"type":"image","attrs":{"imageId":"` + id + `","src":"/img/lib/` + id + `","alt":""}}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := (docnodes.Rules{}).Check(nil, d); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	for _, id := range []string{"architecture-overview", "up-", "up-AAAAAAAAAAAAAAAAAAAAAAAAAA", "up-aaaaaaaaaaaaaaaaaaaaaaaaa1", "../up-aaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if IsUpload(id) {
			t.Errorf("%q is an upload's id", id)
		}
	}
}

// NFR-16: no file makes Process panic, and what it takes decodes.
func FuzzProcess(f *testing.F) {
	f.Add(pngOf(f, picture(16, 16)))
	f.Add(withExif(jpegOf(f, picture(16, 16)), 6, "x"))
	f.Add(pngHeader(64, 64))
	for _, name := range []string{"blue-purple-pink.lossy.webp", "gopher-doc.1bpp.lossless.webp"} {
		b, _ := os.ReadFile("testdata/" + name)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		im, err := Process(context.Background(), b)
		var r *writer.Refusal
		if err != nil && !errors.As(err, &r) {
			t.Fatalf("not a refusal: %v", err)
		}
		if err == nil {
			if _, _, err := image.Decode(bytes.NewReader(im.Data)); err != nil {
				t.Fatalf("stored bytes do not decode: %v", err)
			}
		}
	})
}

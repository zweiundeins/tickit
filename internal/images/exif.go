package images

import "encoding/binary"

// orientation reads the Exif orientation of a JPEG (tag 0x0112 of IFD0 in
// its APP1 segment): 1 to 8, or 0 without one. It reads the bytes as
// untrusted, bounds checked, and stops at the first image data.
func orientation(b []byte) int {
	i := 2 // after SOI
	for i+4 <= len(b) {
		if b[i] != 0xff {
			return 0
		}
		marker := b[i+1]
		if marker == 0xff { // padding
			i++
			continue
		}
		if marker == 0xda || marker == 0xd9 { // start of scan, end of image
			return 0
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return 0
		}
		seg := b[i+4 : i+2+n]
		if marker == 0xe1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + n
	}
	return 0
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 0
	}
	count := int(bo.Uint16(t[ifd:]))
	for k := range count {
		e := ifd + 2 + 12*k
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // SHORT
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

package people

// Names and faces from a hash, ported from lith's board demo
// (site/internal/demos/board/avatar.go, MIT License, Copyright (c) 2026
// zwei und eins gmbh and lith contributors), with the cursor party's
// animals for names in the manner of "Sunny Otter".

import (
	"crypto/sha256"
	"strconv"
	"strings"
)

var (
	adjectives = [32]string{
		"Amber", "Brisk", "Calm", "Deft", "Eager", "Fond", "Glad", "Hardy",
		"Jolly", "Keen", "Lucid", "Merry", "Nimble", "Plucky", "Quick", "Sunny",
		"Bold", "Bright", "Clever", "Cosy", "Daring", "Gentle", "Happy", "Lively",
		"Mellow", "Noble", "Proud", "Quiet", "Swift", "Tidy", "Witty", "Zesty",
	}
	animals = [32]string{
		"Fox", "Owl", "Moth", "Wren", "Hare", "Lynx", "Seal", "Crow",
		"Newt", "Toad", "Mole", "Vole", "Kite", "Yak", "Elk", "Ibex",
		"Crab", "Carp", "Pike", "Wolf", "Bear", "Lark", "Swan", "Duck",
		"Goat", "Mink", "Otter", "Heron", "Finch", "Gecko", "Koala", "Panda",
	}
)

// avatar is what an id alone gives a person: a name, a hue and a face.
type avatar struct {
	Name string
	Hue  int
	Face string
}

// avatarOf draws the avatar of an id. The first 32 bits of the id's
// SHA-256 pick the face (15 bits: three columns of five, the third
// mirrored onto the first two), the hue (4 bits) and the name (10 bits).
func avatarOf(id string) avatar {
	sum := sha256.Sum256([]byte(id))
	n := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	return avatar{
		Name: adjectives[n>>27&31] + " " + animals[n>>22&31],
		Hue:  int(n>>15&15) * 360 / 16,
		Face: face(n & 0x7fff),
	}
}

// face draws 15 bits on a 7x7 grid (a pixel of margin around 5x5),
// mirrored left to right, as a path of a rectangle per run of pixels.
func face(bits uint32) string {
	// A face needs enough pixels to read as one: sparse ones turn over.
	if ones(bits) < 6 {
		bits ^= 0x7fff
	}
	var px [5][5]bool
	for col := 0; col < 3; col++ {
		for row := 0; row < 5; row++ {
			if bits&(1<<(col*5+row)) != 0 {
				px[row][col], px[row][4-col] = true, true
			}
		}
	}
	var p strings.Builder
	for row := range px {
		for x := 0; x < 5; {
			if !px[row][x] {
				x++
				continue
			}
			w := 1
			for x+w < 5 && px[row][x+w] {
				w++
			}
			n := strconv.Itoa(w)
			p.WriteString("M" + strconv.Itoa(x+1) + " " + strconv.Itoa(row+1) + "h" + n + "v1h-" + n + "z")
			x += w
		}
	}
	return p.String()
}

func ones(n uint32) int {
	c := 0
	for ; n != 0; n &= n - 1 {
		c++
	}
	return c
}

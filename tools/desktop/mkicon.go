// Command desktop draws the hub logo (platform/shell/public/favicon.svg: a
// blue rounded tile with four white squares) into a Windows .ico or a macOS .icns, with
// antialiasing, so no image tools are needed.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
)

type rrect struct{ x, y, w, h, r, alpha float64 }

var (
	tile    = rrect{0, 0, 32, 32, 8, 1}
	squares = []rrect{{7, 7, 8, 8, 2, 1}, {17, 7, 8, 8, 2, .7}, {7, 17, 8, 8, 2, .7}, {17, 17, 8, 8, 2, 1}}
	blue    = [3]float64{0x3b, 0x6f, 0xf5}
)

func (r rrect) inside(px, py float64) bool {
	cx, cy := r.x+r.w/2, r.y+r.h/2
	ax, ay := math.Abs(px-cx), math.Abs(py-cy)
	if ax > r.w/2 || ay > r.h/2 {
		return false
	}
	dx, dy := math.Max(ax-(r.w/2-r.r), 0), math.Max(ay-(r.h/2-r.r), 0)
	return dx*dx+dy*dy <= r.r*r.r
}

// Draw renders the logo at size×size pixels (4×4 supersampling).
func Draw(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	scale := 32 / float64(size)
	for y := range size {
		for x := range size {
			var r, g, b, a float64 // premultiplied sums
			for j := range ss {
				for i := range ss {
					px := (float64(x) + (float64(i)+.5)/ss) * scale
					py := (float64(y) + (float64(j)+.5)/ss) * scale
					if !tile.inside(px, py) {
						continue
					}
					c := blue
					for _, s := range squares {
						if s.inside(px, py) {
							for k := range c {
								c[k] = c[k]*(1-s.alpha) + 255*s.alpha
							}
						}
					}
					r, g, b, a = r+c[0], g+c[1], b+c[2], a+1
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / ss / ss)})
		}
	}
	return img
}

// ICO packs PNG images (Vista+ reads PNG entries at every size).
func ICO(sizes []int) ([]byte, error) {
	var pngs [][]byte
	for _, s := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, Draw(s)); err != nil {
			return nil, err
		}
		pngs = append(pngs, buf.Bytes())
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0 // 0 means 256
		}
		binary.Write(&out, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, BPP            uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(pngs[i])), uint32(offset)})
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes(), nil
}

// ICNS packs PNG images into a macOS icon file (OSType + big-endian length per entry).
func ICNS() ([]byte, error) {
	types := []struct {
		kind string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}}
	var body bytes.Buffer
	for _, t := range types {
		var buf bytes.Buffer
		if err := png.Encode(&buf, Draw(t.size)); err != nil {
			return nil, err
		}
		body.WriteString(t.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+buf.Len()))
		body.Write(buf.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/desktop <out.ico|out.icns>")
		os.Exit(2)
	}
	var data []byte
	var err error
	if strings.HasSuffix(os.Args[1], ".icns") {
		data, err = ICNS()
	} else {
		data, err = ICO([]int{16, 24, 32, 48, 64, 128, 256})
	}
	if err == nil {
		err = os.WriteFile(os.Args[1], data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "desktop:", err)
		os.Exit(1)
	}
}

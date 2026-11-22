package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestDraw(t *testing.T) {
	img := Draw(64)
	if a := img.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("rounded corner should be transparent, alpha %d", a)
	}
	if c := img.NRGBAAt(32, 4); c.B != 0xf5 || c.A != 255 {
		t.Errorf("tile edge should be blue: %+v", c)
	}
	if c := img.NRGBAAt(22, 22); c.R != 255 || c.G != 255 {
		t.Errorf("top-left square should be white: %+v", c)
	}
	if c := img.NRGBAAt(42, 22); c.R < 150 || c.R > 220 {
		t.Errorf("top-right square should be 70%% white over blue: %+v", c)
	}
}

func TestICO(t *testing.T) {
	data, err := ICO([]int{16, 256})
	if err != nil {
		t.Fatal(err)
	}
	var hdr [3]uint16
	binary.Read(bytes.NewReader(data), binary.LittleEndian, &hdr)
	if hdr != [3]uint16{0, 1, 2} {
		t.Fatalf("header %v", hdr)
	}
	if data[6] != 16 || data[22] != 0 { // 256 is stored as 0
		t.Errorf("sizes %d %d", data[6], data[22])
	}
	off := binary.LittleEndian.Uint32(data[6+12:])
	if _, err := png.Decode(bytes.NewReader(data[off:])); err != nil {
		t.Errorf("first image isn't a PNG: %v", err)
	}
}

func TestICNS(t *testing.T) {
	data, err := ICNS()
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
		t.Fatalf("bad header")
	}
	if string(data[8:12]) != "icp4" {
		t.Errorf("first entry %q", data[8:12])
	}
	if _, err := png.Decode(bytes.NewReader(data[16 : 8+binary.BigEndian.Uint32(data[12:16])])); err != nil {
		t.Errorf("first entry isn't a PNG: %v", err)
	}
}

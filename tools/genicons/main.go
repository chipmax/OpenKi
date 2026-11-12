// genicons derives every shipped icon from assets/icon-src.png. The master is
// a 1024×1024 RGBA plate; this tool area-filters it down to each target size
// and wraps the PNGs in the ICO containers Windows loads. Run from the repo
// root:
//
//	go run ./tools/genicons
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

type output struct {
	path string
	size int
}

var pngOutputs = []output{
	{"assets/icon-16.png", 16},
	{"assets/icon-32.png", 32},
	{"assets/icon-128.png", 128},
	{"assets/icon-256.png", 256},
	{"internal/window/assets/icon-256.png", 256},
	{"internal/tray/assets/icon-256.png", 256},
	{"internal/gateway/web/favicon.png", 128},
	{"cmd/openkistats/logo.png", 256},
}

var icoOutputs = []struct {
	path  string
	sizes []int
}{
	{"assets/icon.ico", []int{16, 24, 32, 48, 64, 128, 256}},
	{"internal/window/assets/icon.ico", []int{16, 24, 32, 48, 64, 128, 256}},
	{"internal/tray/assets/tray.ico", []int{16, 24, 32, 48}},
}

func main() {
	src, err := load("assets/icon-src.png")
	if err != nil {
		fail(err)
	}
	for _, out := range pngOutputs {
		if err := writePNG(out.path, resize(src, out.size)); err != nil {
			fail(err)
		}
	}
	for _, out := range icoOutputs {
		imgs := make([][]byte, 0, len(out.sizes))
		for _, size := range out.sizes {
			var buf bytes.Buffer
			if err := png.Encode(&buf, resize(src, size)); err != nil {
				fail(err)
			}
			imgs = append(imgs, buf.Bytes())
		}
		if err := writeICO(out.path, out.sizes, imgs); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicons:", err)
	os.Exit(1)
}

func load(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	in, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(in.Bounds())
	draw.Draw(out, out.Bounds(), in, in.Bounds().Min, draw.Src)
	return out, nil
}

// resize area-filters the source into a size×size square. Colours are weighted
// by alpha, so transparent plate corners do not bleed a dark fringe into the
// rounded edge at 16px.
func resize(src *image.NRGBA, size int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		y0 := b.Min.Y + y*sh/size
		y1 := b.Min.Y + (y+1)*sh/size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := range size {
			x0 := b.Min.X + x*sw/size
			x1 := b.Min.X + (x+1)*sw/size
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sumA, sumR, sumG, sumB uint64
			var n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := src.NRGBAAt(sx, sy)
					a := uint64(c.A)
					sumA += a
					sumR += uint64(c.R) * a
					sumG += uint64(c.G) * a
					sumB += uint64(c.B) * a
					n++
				}
			}
			if n == 0 || sumA == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(sumR / sumA),
				G: uint8(sumG / sumA),
				B: uint8(sumB / sumA),
				A: uint8(sumA / n),
			})
		}
	}
	return dst
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(f, img)
}

// writeICO emits a Vista-style icon directory whose entries are PNG-compressed.
// Windows 10/11 and getlantern/systray both accept PNG entries.
func writeICO(path string, sizes []int, blobs [][]byte) error {
	if len(sizes) != len(blobs) || len(sizes) == 0 {
		return fmt.Errorf("ICO size/blob mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, size := range sizes {
		buf.WriteByte(byte(size))
		if size >= 256 {
			buf.WriteByte(0)
		} else {
			buf.WriteByte(byte(size))
		}
		buf.WriteByte(0)                                    // palette count
		buf.WriteByte(0)                                    // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bit count
		binary.Write(&buf, binary.LittleEndian, uint32(len(blobs[i])))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(blobs[i])
	}
	for _, blob := range blobs {
		buf.Write(blob)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

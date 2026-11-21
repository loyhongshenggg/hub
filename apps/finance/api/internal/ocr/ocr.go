// Package ocr turns PDF pages into positioned words. Some bank statements
// (HSBC Singapore) have no text layer — every glyph is an image mask — so
// pages are rendered with poppler's pdftoppm and read with Tesseract.
// Statements never leave the machine.
package ocr

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Word is one OCR'd word with its box in page pixels (at the render DPI).
type Word struct {
	Text           string
	X0, Y0, X1, Y1 int
	Conf           float64
}

// Page is one rendered page.
type Page struct {
	Number        int // 1-based
	Width, Height int // pixels at DPI
	DPI           int
	Words         []Word
}

// Options select how a page is read. Crop is a fraction of the page
// (0..1 on each axis); words come back in full-page coordinates.
type Options struct {
	DPI  int
	PSM  int // Tesseract page segmentation mode (6: one block, 4: columns, 11: sparse)
	Crop *Rect
}

// Rect is a fractional page rectangle.
type Rect struct{ X0, Y0, X1, Y1 float64 }

// Engine reads a PDF into pages.
type Engine interface {
	Read(ctx context.Context, pdfPath string, opt Options) ([]Page, error)
	Available() error
}

// Tesseract runs the poppler + tesseract command-line tools.
type Tesseract struct {
	PdfToPPM  string // default "pdftoppm"
	PdfToText string // default "pdftotext"
	Tesseract string // default "tesseract"
	Workers   int    // pages in parallel; default NumCPU/2 (min 1)
}

func (t *Tesseract) bins() (string, string) {
	a, b := t.PdfToPPM, t.Tesseract
	if a == "" {
		a = "pdftoppm"
	}
	if b == "" {
		b = "tesseract"
	}
	return a, b
}

// Available reports whether both tools are installed.
func (t *Tesseract) Available() error {
	a, b := t.bins()
	var missing []string
	for _, bin := range []string{a, b} {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("OCR tools missing: %s (install poppler-utils and tesseract-ocr)", strings.Join(missing, ", "))
	}
	return nil
}

// Render writes one grayscale PNG per page into dir and returns their paths
// in page order.
func (t *Tesseract) Render(ctx context.Context, pdfPath, dir string, dpi int) ([]string, error) {
	pdftoppm, _ := t.bins()
	prefix := filepath.Join(dir, "p")
	cmd := exec.CommandContext(ctx, pdftoppm, "-r", strconv.Itoa(dpi), "-gray", "-png", pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %v: %s", err, bytes.TrimSpace(out))
	}
	files, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("pdftoppm produced no pages")
	}
	sort.Slice(files, func(i, j int) bool { return pageNum(files[i]) < pageNum(files[j]) })
	return files, nil
}

func pageNum(path string) int {
	base := strings.TrimSuffix(filepath.Base(path), ".png")
	n, _ := strconv.Atoi(base[strings.LastIndexByte(base, '-')+1:])
	return n
}

// Read renders and OCRs every page.
func (t *Tesseract) Read(ctx context.Context, pdfPath string, opt Options) ([]Page, error) {
	if opt.DPI == 0 {
		opt.DPI = 300
	}
	if opt.PSM == 0 {
		opt.PSM = 6
	}
	dir, err := os.MkdirTemp("", "finance-ocr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	files, err := t.Render(ctx, pdfPath, dir, opt.DPI)
	if err != nil {
		return nil, err
	}
	workers := t.Workers
	if workers <= 0 {
		workers = max(1, runtime.NumCPU()/2)
	}
	pages := make([]Page, len(files))
	errs := make([]error, len(files))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := t.page(ctx, f, opt)
			p.Number = i + 1
			pages[i], errs[i] = p, err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return pages, nil
}

func (t *Tesseract) page(ctx context.Context, pngPath string, opt Options) (Page, error) {
	img, err := decodePNG(pngPath)
	if err != nil {
		return Page{}, err
	}
	b := img.Bounds()
	p := Page{Width: b.Dx(), Height: b.Dy(), DPI: opt.DPI}
	src, offX, offY := pngPath, 0, 0
	if opt.Crop != nil {
		r := image.Rect(int(opt.Crop.X0*float64(p.Width)), int(opt.Crop.Y0*float64(p.Height)),
			int(opt.Crop.X1*float64(p.Width)), int(opt.Crop.Y1*float64(p.Height))).Intersect(b)
		sub, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		})
		if !ok || r.Empty() {
			return p, errors.New("cannot crop page image")
		}
		src = strings.TrimSuffix(pngPath, ".png") + "-crop.png"
		if err := encodePNG(src, sub.SubImage(r)); err != nil {
			return p, err
		}
		offX, offY = r.Min.X, r.Min.Y
	}
	_, tess := t.bins()
	cmd := exec.CommandContext(ctx, tess, src, "stdout", "--psm", strconv.Itoa(opt.PSM),
		"--dpi", strconv.Itoa(opt.DPI), "-c", "preserve_interword_spaces=1", "tsv")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return p, fmt.Errorf("tesseract: %v: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	words, err := ParseTSV(out)
	if err != nil {
		return p, err
	}
	for i := range words {
		words[i].X0 += offX
		words[i].X1 += offX
		words[i].Y0 += offY
		words[i].Y1 += offY
	}
	p.Words = words
	return p, nil
}

// ParseTSV reads Tesseract's TSV output and returns the word rows (level 5).
func ParseTSV(data []byte) ([]Word, error) {
	var words []Word
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	header := true
	for sc.Scan() {
		if header {
			header = false
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		left, _ := strconv.Atoi(f[6])
		top, _ := strconv.Atoi(f[7])
		w, _ := strconv.Atoi(f[8])
		h, _ := strconv.Atoi(f[9])
		conf, _ := strconv.ParseFloat(f[10], 64)
		words = append(words, Word{Text: text, X0: left, Y0: top, X1: left + w, Y1: top + h, Conf: conf})
	}
	return words, sc.Err()
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func encodePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// TextLayer returns the PDF's own words with their boxes (pdftotext -bbox),
// scaled to 300 dpi pixels so layout thresholds match OCR output. Image-only
// PDFs return pages without words.
func (t *Tesseract) TextLayer(ctx context.Context, pdfPath string) ([]Page, error) {
	bin := t.PdfToText
	if bin == "" {
		bin = "pdftotext"
	}
	cmd := exec.CommandContext(ctx, bin, "-bbox", pdfPath, "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %v: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return ParseBBox(out, 300), nil
}

var (
	rePageTag = regexp.MustCompile(`<page width="([\d.]+)" height="([\d.]+)">`)
	reWordTag = regexp.MustCompile(`<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)">([^<]*)</word>`)
)

// ParseBBox reads pdftotext -bbox XHTML (coordinates in points).
func ParseBBox(data []byte, dpi int) []Page {
	scale := float64(dpi) / 72
	px := func(s string) int {
		f, _ := strconv.ParseFloat(s, 64)
		return int(f * scale)
	}
	var pages []Page
	for _, chunk := range strings.Split(string(data), "<page ")[1:] {
		m := rePageTag.FindStringSubmatch("<page " + chunk)
		if m == nil {
			continue
		}
		p := Page{Number: len(pages) + 1, Width: px(m[1]), Height: px(m[2]), DPI: dpi}
		for _, w := range reWordTag.FindAllStringSubmatch(chunk, -1) {
			text := html.UnescapeString(strings.TrimSpace(w[5]))
			if text == "" {
				continue
			}
			p.Words = append(p.Words, Word{Text: text, X0: px(w[1]), Y0: px(w[2]), X1: px(w[3]), Y1: px(w[4]), Conf: 100})
		}
		p.Words = dropVertical(p.Words)
		pages = append(pages, p)
	}
	return pages
}

// dropVertical removes rotated text (print codes running up the margin,
// "EPSTCSX / 000246 - 3 OF 8"). Its words are taller than wide and share one
// narrow x-band; every word in such a band goes, single characters included.
func dropVertical(ws []Word) []Word {
	type band struct{ x0, x1 int }
	var bands []band
	for _, w := range ws {
		if utf8.RuneCountInString(w.Text) >= 2 && w.Y1-w.Y0 > 3*(w.X1-w.X0)/2 {
			bands = append(bands, band{w.X0, w.X1})
		}
	}
	if len(bands) == 0 {
		return ws
	}
	out := ws[:0]
	for _, w := range ws {
		vertical := false
		for _, b := range bands {
			if abs(w.X0-b.x0) <= 3 && abs(w.X1-b.x1) <= 3 {
				vertical = true
				break
			}
		}
		if !vertical {
			out = append(out, w)
		}
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// RenderPage renders a single page to PNG bytes (for the review screen).
func (t *Tesseract) RenderPage(ctx context.Context, pdfPath string, page, dpi int) ([]byte, error) {
	pdftoppm, _ := t.bins()
	n := strconv.Itoa(page)
	cmd := exec.CommandContext(ctx, pdftoppm, "-r", strconv.Itoa(dpi), "-f", n, "-l", n, "-singlefile", "-png", pdfPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdftoppm: %v: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

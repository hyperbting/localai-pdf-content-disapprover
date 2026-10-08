// Package pdftext loads a PDF and extracts plain text per page.
package pdftext

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

type Page struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type Document struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Pages  []Page `json:"pages"`
}

// Text joins all pages with page markers.
func (d *Document) Text() string {
	var b strings.Builder
	for _, p := range d.Pages {
		fmt.Fprintf(&b, "--- Page %d ---\n%s\n", p.Number, p.Text)
	}
	return b.String()
}

// Load opens path and extracts text from every page.
func Load(path string) (doc *Document, err error) {
	sum, err := fileSHA256(path)
	if err != nil {
		return nil, err
	}

	// The underlying parser panics on some malformed files; surface that as an error.
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, fmt.Errorf("parse %s: %v", path, r)
		}
	}()

	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	doc = &Document{Path: path, SHA256: sum}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i, err)
		}
		doc.Pages = append(doc.Pages, Page{Number: i, Text: strings.TrimSpace(text)})
	}
	return doc, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

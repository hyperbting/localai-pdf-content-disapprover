package pdftext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, BuildTestPDF("Hello world", "Second page"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(doc.Pages))
	}
	if !strings.Contains(doc.Pages[0].Text, "Hello world") || !strings.Contains(doc.Pages[1].Text, "Second page") {
		t.Fatalf("unexpected text: %+v", doc.Pages)
	}
	if len(doc.SHA256) != 64 {
		t.Fatalf("bad sha256 %q", doc.SHA256)
	}
}

func TestLoadInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pdf")
	os.WriteFile(path, []byte("not a pdf"), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

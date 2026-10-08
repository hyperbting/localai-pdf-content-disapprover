package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/review"
	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

// fakeAI disapproves anything that mentions a password.
var fakeAI = ai.ClientFunc(func(_ context.Context, req ai.Request) (string, error) {
	if strings.Contains(req.Messages[len(req.Messages)-1].Content, "Password: hunter2") {
		return `{"verdict":"disapprove","findings":[{"page":1,"category":"credential","severity":"high","excerpt":"Password","reason":"contains a password"}]}`, nil
	}
	return `{"verdict":"approve","findings":[]}`, nil
})

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(WithClient(fakeAI))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func writePDF(t *testing.T, dir string, pages ...string) string {
	t.Helper()
	p := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(p, pdftext.BuildTestPDF(pages...), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReviewApprove(t *testing.T) {
	pdf := writePDF(t, t.TempDir(), "Quarterly results are good")
	out, err := run(t, "review", pdf, "-q", "--fail-on-disapprove")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.HasPrefix(out, "APPROVED") {
		t.Fatalf("output: %s", out)
	}
}

func TestReviewDisapproveExitCode(t *testing.T) {
	pdf := writePDF(t, t.TempDir(), "Password: hunter2")
	out, err := run(t, "review", pdf, "-q", "--fail-on-disapprove")
	var ec exitCode
	if !errors.As(err, &ec) || ec != 2 {
		t.Fatalf("err = %v, want exit 2\n%s", err, out)
	}
	if !strings.Contains(out, "DISAPPROVED") {
		t.Fatalf("output: %s", out)
	}
}

func TestReviewSaveAndCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	dir := t.TempDir()
	pdf := writePDF(t, dir, "Password: hunter2")
	report := filepath.Join(dir, "reviews", "doc.json")

	out, err := run(t, "review", pdf, "-q", "-o", report, "--commit", "--git-init")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	var rep review.Report
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != review.Disapprove || len(rep.Findings) != 1 {
		t.Fatalf("report: %+v", rep)
	}

	log, err := exec.Command("git", "-C", filepath.Dir(report), "log", "--format=%s").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "Review doc.pdf: disapprove (1 findings)") {
		t.Fatalf("git log: %s", log)
	}
}

func TestReviewWithLawsExamplesAndReasonFormat(t *testing.T) {
	dir := t.TempDir()
	pdf := writePDF(t, dir, "Patient record: diabetes")
	law := filepath.Join(dir, "pdpa.txt")
	ok := filepath.Join(dir, "ok.txt")
	tmpl := filepath.Join(dir, "reason.txt")
	os.WriteFile(law, []byte("Personal Data Protection Act\nArticle 6: medical records shall not be collected."), 0o644)
	os.WriteFile(ok, []byte("Our clinic opens at 9am."), 0o644)
	os.WriteFile(tmpl, []byte("Against {law} {article}: {reason} (page {page})"), 0o644)
	report := filepath.Join(dir, "r.json")

	var prompt string
	client := ai.ClientFunc(func(_ context.Context, req ai.Request) (string, error) {
		prompt = req.Messages[len(req.Messages)-1].Content
		return `{"verdict":"disapprove","findings":[{"page":1,"rule_id":"PDPA-6","law":"Personal Data Protection Act","article":"Article 6","category":"medical","severity":"high","excerpt":"diabetes","reason":"discloses a medical condition"}]}`, nil
	})
	root := NewRootCmd(WithClient(client))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"review", pdf, "-q", "--laws", law, "--pass-examples", ok, "--reason-format", tmpl, "-o", report})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}

	for _, want := range []string{"Article 6: medical records", "## APPROVED EXAMPLES", "clinic opens at 9am"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	wantMsg := "Against Personal Data Protection Act Article 6: discloses a medical condition (page 1)"
	if !strings.Contains(out.String(), wantMsg) {
		t.Errorf("output missing rendered reason:\n%s", out.String())
	}

	var rep review.Report
	data, _ := os.ReadFile(report)
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	f := rep.Findings[0]
	if f.RuleID != "PDPA-6" || f.Article != "Article 6" || f.Message != wantMsg {
		t.Errorf("finding: %+v", f)
	}
	if len(rep.Policy) != 3 {
		t.Errorf("policy sources: %+v", rep.Policy)
	}
}

func TestProvidersDetect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"qwen"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"providers", "--detect", "-e", srv.URL})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "provider: openai") || !strings.Contains(out.String(), "model:    qwen") {
		t.Errorf("output: %s", out.String())
	}
}

func TestCommitRequiresOut(t *testing.T) {
	pdf := writePDF(t, t.TempDir(), "x")
	if _, err := run(t, "extract", pdf, "--commit"); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtract(t *testing.T) {
	pdf := writePDF(t, t.TempDir(), "Page one", "Page two")
	out, err := run(t, "extract", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- Page 2 ---") || !strings.Contains(out, "Page two") {
		t.Fatalf("output: %s", out)
	}
}

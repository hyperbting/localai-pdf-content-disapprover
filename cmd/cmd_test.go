package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

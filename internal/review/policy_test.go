package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPolicy(t *testing.T) {
	dir := t.TempDir()
	law := write(t, dir, "pdpa.txt", "\xef\xbb\xbf個人資料保護法\n第6條 有關病歷、醫療之個人資料，不得蒐集。")
	ok := write(t, dir, "ok.txt", "本產品通過 ISO 9001 認證。")
	tmpl := write(t, dir, "reason.txt", "違反{law}{article}：{reason}（第{page}頁）\n")

	p, err := LoadPolicy(PolicyFiles{Laws: []string{law}, PassExamples: []string{ok}, ReasonFormat: tmpl})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(p.Laws[0].Text, "\ufeff") {
		t.Error("BOM not stripped")
	}
	text := p.Text()
	for _, want := range []string{"## LAWS", "LAW FILE: pdpa.txt", "第6條", "## APPROVED EXAMPLES", "ISO 9001"} {
		if !strings.Contains(text, want) {
			t.Errorf("policy text missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "builtin") {
		t.Error("built-in rules must not be used when laws are given")
	}

	kinds := map[string]bool{}
	for _, s := range p.Sources() {
		kinds[s.Kind] = true
		if s.SHA256 == "" {
			t.Errorf("source %+v has no hash", s)
		}
	}
	if !kinds[KindLaw] || !kinds[KindPassExample] || !kinds[KindReasonFormat] || kinds[KindBuiltin] {
		t.Errorf("sources = %+v", p.Sources())
	}

	got := Render(p.ReasonFormat, Finding{Page: 2, Law: "個人資料保護法", Article: "第6條", Reason: "含有病歷資料"})
	if want := "違反個人資料保護法第6條：含有病歷資料（第2頁）"; got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestLoadPolicyDefaultsAndErrors(t *testing.T) {
	p, err := LoadPolicy(PolicyFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Text(), DefaultRules) || p.Sources()[0].Kind != KindBuiltin {
		t.Error("empty policy should use built-in rules")
	}
	empty := write(t, t.TempDir(), "empty.txt", "  \n")
	if _, err := LoadPolicy(PolicyFiles{Laws: []string{empty}}); err == nil {
		t.Error("expected error for empty law file")
	}
	if _, err := LoadPolicy(PolicyFiles{Rules: []string{"missing.txt"}}); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRenderDefaultFormatAndCitation(t *testing.T) {
	cases := []struct {
		f    Finding
		want string
	}{
		{Finding{Page: 3, Severity: "high", Law: "Fair Trade Act", Article: "Art. 21", Reason: "misleading claim"}, "p.3 [high] Fair Trade Act Art. 21: misleading claim"},
		{Finding{Page: 1, Severity: "low", RuleID: "R2", Reason: "internal marking"}, "p.1 [low] R2: internal marking"},
		{Finding{Page: 1, Severity: "high", Reason: "x"}, "p.1 [high] unspecified: x"},
	}
	for _, c := range cases {
		if got := Render("", c.f); got != c.want {
			t.Errorf("Render = %q, want %q", got, c.want)
		}
	}
}

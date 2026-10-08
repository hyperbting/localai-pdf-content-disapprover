package review

import (
	"context"
	"strings"
	"testing"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

func TestParseVerdict(t *testing.T) {
	cases := map[string]string{
		`{"verdict":"approve","findings":[]}`:                              Approve,
		"```json\n{\"verdict\":\"Disapprove\",\"findings\":[]}\n```":       Disapprove,
		`Sure! {"verdict":"?","findings":[{"page":1,"reason":"x"}]} done.`: Disapprove,
		"<think>maybe {not json}</think>\n{\"verdict\":\"approve\"}":       Approve,
	}
	for in, want := range cases {
		v, err := ParseVerdict(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if v.Verdict != want {
			t.Errorf("%q: verdict %q, want %q", in, v.Verdict, want)
		}
	}
	for _, bad := range []string{"no json", `{"verdict":"maybe"}`} {
		if _, err := ParseVerdict(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestDisapproveWithoutFindingsGetsReason(t *testing.T) {
	doc := &pdftext.Document{Pages: []pdftext.Page{{Number: 4, Text: "x"}}}
	client := ai.ClientFunc(func(context.Context, ai.Request) (string, error) {
		return `{"verdict":"disapprove","findings":[]}`, nil
	})
	rep, err := (&Reviewer{Client: client}).Review(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != Disapprove || len(rep.Findings) != 1 {
		t.Fatalf("report: %+v", rep)
	}
	f := rep.Findings[0]
	if f.Page != 4 || !strings.Contains(f.Message, "without citing") {
		t.Errorf("finding: %+v", f)
	}
}

func TestReviewChunksAndAggregates(t *testing.T) {
	doc := &pdftext.Document{Pages: []pdftext.Page{
		{Number: 1, Text: strings.Repeat("a", 50)},
		{Number: 2, Text: "secret password"},
		{Number: 3, Text: strings.Repeat("c", 50)},
	}}
	calls := 0
	client := ai.ClientFunc(func(_ context.Context, req ai.Request) (string, error) {
		calls++
		if strings.Contains(req.Messages[1].Content, "secret password") {
			return `{"verdict":"disapprove","findings":[{"page":2,"category":"pii","severity":"high","excerpt":"password","reason":"credential"}]}`, nil
		}
		return `{"verdict":"approve","findings":[]}`, nil
	})

	rep, err := (&Reviewer{Client: client, MaxChars: 80}).Review(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (one per page at this chunk size)", calls)
	}
	if rep.Verdict != Disapprove || len(rep.Findings) != 1 || rep.Findings[0].Page != 2 {
		t.Errorf("unexpected report: %+v", rep)
	}
}

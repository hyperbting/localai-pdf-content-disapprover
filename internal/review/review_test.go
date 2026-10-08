package review

import (
	"context"
	"errors"
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

func TestRetryOnInvalidReply(t *testing.T) {
	doc := &pdftext.Document{Pages: []pdftext.Page{{Number: 1, Text: "hello"}}}
	var requests [][]ai.Message
	client := ai.ClientFunc(func(_ context.Context, req ai.Request) (string, error) {
		requests = append(requests, req.Messages)
		if len(requests) == 1 {
			return "Sure, the document looks fine to me.", nil
		}
		return `{"verdict":"approve","findings":[]}`, nil
	})
	var retried []int
	rv := &Reviewer{Client: client, Retries: 1, OnRetry: func(_, _, attempt int, _ error) { retried = append(retried, attempt) }}
	rep, err := rv.Review(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != Approve || len(requests) != 2 || len(retried) != 1 {
		t.Fatalf("verdict %s, %d requests, retries %v", rep.Verdict, len(requests), retried)
	}
	// The retry shows the model its bad reply and the parse error.
	second := requests[1]
	if len(second) != 4 || second[2].Role != "assistant" || second[2].Content != "Sure, the document looks fine to me." ||
		!strings.Contains(second[3].Content, "no JSON object") {
		t.Errorf("retry messages: %+v", second)
	}
}

func TestRetryGivesUp(t *testing.T) {
	doc := &pdftext.Document{Pages: []pdftext.Page{{Number: 3, Text: "hello"}}}
	calls := 0
	client := ai.ClientFunc(func(context.Context, ai.Request) (string, error) {
		calls++
		return "not json", nil
	})
	_, err := (&Reviewer{Client: client, Retries: 2}).Review(context.Background(), doc)
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") || !strings.Contains(err.Error(), "pages 3-3") {
		t.Fatalf("err = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}

	calls = 0
	if _, err := (&Reviewer{Client: client}).Review(context.Background(), doc); err == nil || calls != 1 {
		t.Errorf("Retries 0: err = %v, calls = %d", err, calls)
	}
}

func TestTransportErrorNotRetried(t *testing.T) {
	doc := &pdftext.Document{Pages: []pdftext.Page{{Number: 1, Text: "hello"}}}
	calls := 0
	client := ai.ClientFunc(func(context.Context, ai.Request) (string, error) {
		calls++
		return "", errors.New("connection refused")
	})
	if _, err := (&Reviewer{Client: client, Retries: 3}).Review(context.Background(), doc); err == nil || calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls)
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

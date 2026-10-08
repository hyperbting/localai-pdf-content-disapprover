// Package review asks an ai.Client to approve or disapprove PDF content
// against a set of rules.
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

const (
	Approve    = "approve"
	Disapprove = "disapprove"
)

// DefaultRules is used when no --rules file is given.
const DefaultRules = `Disapprove the document if it contains any of:
- Personally identifiable information (national IDs, full card numbers, passwords, private addresses)
- Confidential or internal-only markings
- Hate speech, harassment, or threats
- Sexually explicit content
- Instructions facilitating violence or illegal activity
Otherwise approve.`

const systemPrompt = `You are a strict document content reviewer.
Evaluate the document excerpt against the RULES.
Reply with ONLY a JSON object, no prose, in exactly this shape:
{"verdict":"approve"|"disapprove","findings":[{"page":<int>,"category":"<short label>","severity":"low"|"medium"|"high","excerpt":"<short quote>","reason":"<why it violates>"}]}
Use "approve" with an empty findings array when nothing violates the rules.`

type Finding struct {
	Page     int    `json:"page"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Excerpt  string `json:"excerpt"`
	Reason   string `json:"reason"`
}

type Report struct {
	File       string    `json:"file"`
	SHA256     string    `json:"sha256"`
	Pages      int       `json:"pages"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	Verdict    string    `json:"verdict"`
	Findings   []Finding `json:"findings"`
	Rules      string    `json:"rules"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

func (r *Report) Approved() bool { return r.Verdict == Approve }

type Reviewer struct {
	Client   ai.Client
	Rules    string
	MaxChars int // max characters of document text per model call
	// Progress, if set, is called before each chunk is sent.
	Progress func(chunk, total int, firstPage, lastPage int)
}

type chunk struct {
	first, last int
	text        string
}

// Review sends the document in page-aligned chunks and aggregates verdicts:
// any disapproving chunk disapproves the whole document.
func (rv *Reviewer) Review(ctx context.Context, doc *pdftext.Document) (*Report, error) {
	rules := rv.Rules
	if strings.TrimSpace(rules) == "" {
		rules = DefaultRules
	}
	rep := &Report{
		File:       doc.Path,
		SHA256:     doc.SHA256,
		Pages:      len(doc.Pages),
		Verdict:    Approve,
		Findings:   []Finding{},
		Rules:      rules,
		ReviewedAt: time.Now().UTC(),
	}

	chunks := split(doc.Pages, rv.MaxChars)
	for i, c := range chunks {
		if rv.Progress != nil {
			rv.Progress(i+1, len(chunks), c.first, c.last)
		}
		reply, err := rv.Client.Chat(ctx, ai.Request{
			JSON: true,
			Messages: []ai.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: fmt.Sprintf("RULES:\n%s\n\nDOCUMENT (pages %d-%d):\n%s", rules, c.first, c.last, c.text)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("pages %d-%d: %w", c.first, c.last, err)
		}
		v, err := ParseVerdict(reply)
		if err != nil {
			return nil, fmt.Errorf("pages %d-%d: %w", c.first, c.last, err)
		}
		if v.Verdict == Disapprove {
			rep.Verdict = Disapprove
		}
		rep.Findings = append(rep.Findings, v.Findings...)
	}
	return rep, nil
}

func split(pages []pdftext.Page, maxChars int) []chunk {
	if maxChars <= 0 {
		maxChars = 8000
	}
	var out []chunk
	var cur *chunk
	for _, p := range pages {
		if strings.TrimSpace(p.Text) == "" {
			continue
		}
		block := fmt.Sprintf("--- Page %d ---\n%s\n", p.Number, p.Text)
		if cur != nil && len(cur.text)+len(block) > maxChars {
			out = append(out, *cur)
			cur = nil
		}
		if cur == nil {
			cur = &chunk{first: p.Number}
		}
		cur.last = p.Number
		cur.text += block
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

type verdict struct {
	Verdict  string    `json:"verdict"`
	Findings []Finding `json:"findings"`
}

// ParseVerdict extracts the JSON object from a model reply, tolerating code
// fences or chatter around it.
func ParseVerdict(reply string) (*verdict, error) {
	// Reasoning models (Qwen3, DeepSeek-R1, Strata) may leak their thinking
	// into the content; drop everything up to the last closing tag.
	if i := strings.LastIndex(reply, "</think>"); i >= 0 {
		reply = reply[i+len("</think>"):]
	}
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("model reply has no JSON object: %q", truncate(reply, 200))
	}
	var v verdict
	if err := json.Unmarshal([]byte(reply[start:end+1]), &v); err != nil {
		return nil, fmt.Errorf("model reply is not valid verdict JSON: %w: %q", err, truncate(reply, 200))
	}
	v.Verdict = strings.ToLower(strings.TrimSpace(v.Verdict))
	switch v.Verdict {
	case Approve, Disapprove:
	default:
		// Be conservative: findings without a clear verdict count as disapproval.
		if len(v.Findings) > 0 {
			v.Verdict = Disapprove
		} else {
			return nil, fmt.Errorf("model returned unknown verdict %q", v.Verdict)
		}
	}
	return &v, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

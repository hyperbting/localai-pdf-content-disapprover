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

const systemPrompt = `You are a strict document compliance reviewer.
Check the DOCUMENT against the RULES and LAWS in the POLICY. Content that is against any law or breaks any rule must be disapproved. Content like the APPROVED EXAMPLES is acceptable.
Reply with ONLY a JSON object, no prose, in exactly this shape:
{"verdict":"approve"|"disapprove","findings":[{"page":<int>,"rule_id":"<identifier of the rule or law broken>","law":"<law name, empty for house rules>","article":"<article or section as written, or empty>","category":"<short label>","severity":"low"|"medium"|"high","excerpt":"<exact short quote from the document>","reason":"<one sentence: why this content breaks that rule or law>"}]}
Every finding must cite the specific rule or law it breaks, and only rules and laws given in the POLICY. Write reasons in the same language as the policy.
Use "approve" with an empty findings array when nothing breaks the policy.`

type Finding struct {
	Page     int    `json:"page"`
	RuleID   string `json:"rule_id"`
	Law      string `json:"law"`
	Article  string `json:"article"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Excerpt  string `json:"excerpt"`
	Reason   string `json:"reason"`
	// Message is the finding rendered with the policy's reason format.
	Message string `json:"message"`
}

// Citation is "law article", falling back to the rule ID.
func (f Finding) Citation() string {
	if c := strings.TrimSpace(f.Law + " " + f.Article); c != "" {
		return c
	}
	if f.RuleID != "" {
		return f.RuleID
	}
	return "unspecified"
}

type Report struct {
	File       string    `json:"file"`
	SHA256     string    `json:"sha256"`
	Pages      int       `json:"pages"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	Verdict    string    `json:"verdict"`
	Findings   []Finding `json:"findings"`
	Policy     []Source  `json:"policy"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

func (r *Report) Approved() bool { return r.Verdict == Approve }

type Reviewer struct {
	Client ai.Client
	// Policy to judge against; nil uses the built-in rules.
	Policy   *Policy
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
	policy := rv.Policy
	if policy == nil {
		policy = &Policy{ReasonFormat: DefaultReasonFormat}
	}
	policyText := policy.Text()
	rep := &Report{
		File:       doc.Path,
		SHA256:     doc.SHA256,
		Pages:      len(doc.Pages),
		Verdict:    Approve,
		Findings:   []Finding{},
		Policy:     policy.Sources(),
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
				{Role: "user", Content: fmt.Sprintf("POLICY:\n%s\n\nDOCUMENT (pages %d-%d):\n%s", policyText, c.first, c.last, c.text)},
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
			if len(v.Findings) == 0 {
				// A disapproval must always carry a reason; keep the verdict but make the gap visible.
				v.Findings = []Finding{{Category: "unspecified", Severity: "high", Reason: "model disapproved without citing a rule or law"}}
			}
		}
		for _, f := range v.Findings {
			if f.Page == 0 {
				f.Page = c.first
			}
			f.Message = Render(policy.ReasonFormat, f)
			rep.Findings = append(rep.Findings, f)
		}
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

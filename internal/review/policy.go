package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Source kinds recorded in a report so a reviewer can tell exactly which
// policy files produced a verdict.
const (
	KindBuiltin      = "builtin_rules"
	KindRules        = "rules"
	KindLaw          = "law"
	KindPassExample  = "pass_example"
	KindReasonFormat = "reason_format"
)

// DefaultReasonFormat renders a finding when no --reason-format is given.
const DefaultReasonFormat = "p.{page} [{severity}] {citation}: {reason}"

// Doc is one policy text file.
type Doc struct {
	Name   string
	Path   string
	Text   string
	SHA256 string
}

// Source identifies a policy input in the report.
type Source struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// Policy is everything the model judges the document against.
type Policy struct {
	Rules        []Doc
	Laws         []Doc
	PassExamples []Doc
	// ReasonFormat is a template for Finding.Message; see Render.
	ReasonFormat       string
	reasonFormatSource *Source
}

// PolicyFiles lists the local files to build a Policy from.
type PolicyFiles struct {
	Rules        []string
	Laws         []string
	PassExamples []string
	ReasonFormat string
}

// LoadPolicy reads every file in pf. Files must be UTF-8 text; a leading BOM
// (as written by Windows Notepad) is removed.
func LoadPolicy(pf PolicyFiles) (*Policy, error) {
	p := &Policy{ReasonFormat: DefaultReasonFormat}
	var err error
	if p.Rules, err = loadDocs(pf.Rules); err != nil {
		return nil, err
	}
	if p.Laws, err = loadDocs(pf.Laws); err != nil {
		return nil, err
	}
	if p.PassExamples, err = loadDocs(pf.PassExamples); err != nil {
		return nil, err
	}
	if pf.ReasonFormat != "" {
		d, err := loadDoc(pf.ReasonFormat)
		if err != nil {
			return nil, err
		}
		p.ReasonFormat = d.Text
		p.reasonFormatSource = &Source{Kind: KindReasonFormat, Path: d.Path, SHA256: d.SHA256}
	}
	return p, nil
}

func loadDocs(paths []string) ([]Doc, error) {
	docs := make([]Doc, 0, len(paths))
	for _, path := range paths {
		d, err := loadDoc(path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

func loadDoc(path string) (Doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, err
	}
	sum := sha256.Sum256(b)
	text := strings.TrimSpace(string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	if text == "" {
		return Doc{}, fmt.Errorf("%s is empty", path)
	}
	return Doc{Name: filepath.Base(path), Path: path, Text: text, SHA256: hex.EncodeToString(sum[:])}, nil
}

func (p *Policy) usesBuiltin() bool { return len(p.Rules) == 0 && len(p.Laws) == 0 }

// Sources lists the policy inputs for the report.
func (p *Policy) Sources() []Source {
	var out []Source
	if p.usesBuiltin() {
		out = append(out, Source{Kind: KindBuiltin})
	}
	add := func(kind string, docs []Doc) {
		for _, d := range docs {
			out = append(out, Source{Kind: kind, Path: d.Path, SHA256: d.SHA256})
		}
	}
	add(KindRules, p.Rules)
	add(KindLaw, p.Laws)
	add(KindPassExample, p.PassExamples)
	if p.reasonFormatSource != nil {
		out = append(out, *p.reasonFormatSource)
	}
	return out
}

// Text renders the policy section of the prompt.
func (p *Policy) Text() string {
	var b strings.Builder
	if p.usesBuiltin() {
		fmt.Fprintf(&b, "## RULES\n### builtin\n%s\n\n", DefaultRules)
	} else if len(p.Rules) > 0 {
		b.WriteString("## RULES\nHouse rules. Content that breaks any rule must be disapproved. Cite the rule's identifier as written, or the file name and line if it has none.\n\n")
		for _, d := range p.Rules {
			fmt.Fprintf(&b, "### RULES FILE: %s\n%s\n\n", d.Name, d.Text)
		}
	}
	if len(p.Laws) > 0 {
		b.WriteString("## LAWS\nContent that is against any of these laws must be disapproved. Cite the law's name and the article or section number exactly as written.\n\n")
		for _, d := range p.Laws {
			fmt.Fprintf(&b, "### LAW FILE: %s\n%s\n\n", d.Name, d.Text)
		}
	}
	if len(p.PassExamples) > 0 {
		b.WriteString("## APPROVED EXAMPLES\nThese examples are acceptable and must be approved. Do not disapprove content only because it resembles them; use them to calibrate what is allowed.\n\n")
		for _, d := range p.PassExamples {
			fmt.Fprintf(&b, "### EXAMPLE: %s\n%s\n\n", d.Name, d.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// Render fills a reason template with a finding's fields. Placeholders:
// {page} {rule_id} {law} {article} {citation} {category} {severity}
// {excerpt} {reason}. {citation} is "law article", falling back to rule_id.
func Render(tmpl string, f Finding) string {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultReasonFormat
	}
	return strings.TrimSpace(strings.NewReplacer(
		"{page}", strconv.Itoa(f.Page),
		"{rule_id}", f.RuleID,
		"{law}", f.Law,
		"{article}", f.Article,
		"{citation}", f.Citation(),
		"{category}", f.Category,
		"{severity}", f.Severity,
		"{excerpt}", f.Excerpt,
		"{reason}", f.Reason,
	).Replace(tmpl))
}

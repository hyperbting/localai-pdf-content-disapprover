# Handoff: disapprover

Status as of 2026-10-08, at commit `c0d1be8` on `main` (in sync with `origin/main`, working tree clean).

## What it is

`disapprover` is a Go CLI built with Cobra. It extracts the text of a PDF, has a local AI review it against rules, laws and approved examples, and returns **approve** or **disapprove** with cited findings. It can write the report to a file and commit that file to git. See [README.md](../README.md) for usage. A Traditional Chinese version is in [README.zh-TW.md](../README.zh-TW.md).

- Module: `github.com/hyperbting/localai-pdf-content-disapprover`
- Go: 1.26.5
- Dependencies: `spf13/cobra`, `ledongthuc/pdf`
- Size: about 2,400 lines of Go including tests

## Current state

- `go vet ./...` is clean.
- `go test ./...` passes in `cmd`, `internal/pdftext`, `internal/review` and `pkg/ai`.
- No TODO or FIXME comments in the code.

### History (all on 2026-10-08)

| Commit | Change |
|---|---|
| `86869f6` | First version of the CLI: `extract`, `review`, `providers`; ollama, openai and exec backends; save and commit |
| `2e6adb0` | Support for reasoning models (Strata): `--reasoning-effort`, removal of leaked `</think>` text |
| `934dc66` | `--laws`, `--pass-examples`, `--reason-format`; cited findings; `--provider auto` (default) |
| `ec7f526` | Tests for the exec provider |
| `2523124` | `--exec-arg`, so exec paths and arguments can contain spaces |
| `c0d1be8` | When a reply isn't valid verdict JSON, the reply and the parse error are sent back to the model (`--retries`, default 1) |

## How it fits together

```
PDF ──pdftext──▶ pages ──review.split──▶ page-aligned chunks (--max-chars, default 8000)
                                              │
               policy (rules/laws/examples) ──┤  sent again with every chunk
                                              ▼
                                   ai.Client.Chat  (auto | ollama | openai | exec | injected)
                                              │
                         ParseVerdict ◀───────┘  retries with the parse error on bad JSON
                                              │
             Report (disapproved if any chunk is) ──store──▶ -o file, optional git commit
```

| Package | Responsibility | Where to start |
|---|---|---|
| `cmd/` | Cobra commands, global flags and env vars, save and commit flags | `cmd/root.go`, `cmd/review.go` |
| `pkg/ai/` | `Client` interface, provider registry, the auto-detect probe, and the HTTP and exec backends | `pkg/ai/ai.go`, `pkg/ai/auto.go` |
| `internal/pdftext/` | Loading the PDF, extracting text per page, file SHA-256 | `pdftext.go` |
| `internal/review/` | Policy files, chunking, the system prompt, verdict parsing, the retry loop | `review.go` (`Review`, `ask`, `ParseVerdict`), `policy.go` |
| `internal/store/` | Writing files and committing only the given paths with git | `store.go` |

Behaviour that is easy to miss:

- The whole document is disapproved if any one chunk is disapproved.
- A disapproval with no findings gets a finding marked "unspecified", so every disapproval shows a reason.
- The report's `policy` field records the SHA-256 of each rules, laws and examples file that was used.
- If the policy is over 24,000 characters, the CLI prints a context-window warning.
- Exit codes: `0` means OK, `1` means an error, and `2` means disapproved (only with `--fail-on-disapprove`).

## Known gaps and suggested next steps

1. **No tests for `internal/store`.** Writing files and running `git commit` are tested only indirectly through `cmd`. Add a test that commits into a temporary repository.
2. **Scanned PDFs** have no text layer, so the review sees nothing. Options: detect pages with no text and warn or fail, or add an OCR step.
3. **Policy size.** The full policy is sent with every chunk. Large law files use up the context window and slow down long documents. Consider retrieving only the relevant articles, or caching the policy as a prefix where the server supports it.
4. **No end-to-end test against a real model.** The provider tests use `httptest` stubs. A manual smoke run against Ollama or Strata is still the only check of real model output quality.
5. **`--exec` without `--exec-arg`** splits on whitespace, and no shell is involved. Pipes and redirects need a wrapper script. This is documented and deliberate.
6. **No release or CI setup.** There are no GitHub Actions or goreleaser config. Adding a `go test` workflow would be cheap.

## Quick start for the next person

```sh
go build -o disapprover .
go test ./...
./disapprover providers --detect            # which local server would auto use
./disapprover review some.pdf -o reviews/some.json
```

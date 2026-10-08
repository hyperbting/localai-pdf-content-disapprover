# disapprover

English | [繁體中文](README.zh-TW.md)

A Cobra CLI that loads a PDF, has a **local AI** check its content against your rules, and returns **approve** or **disapprove**. You can save the report and commit it to git.

```sh
go build -o disapprover .
```

## Commands

| Command | What it does |
|---|---|
| `extract <pdf>` | Prints the PDF's text, or saves it with `-o`. Add `--json` for per-page JSON with the file's SHA-256. |
| `review <pdf>` | Sends the text to the AI in page-aligned chunks. The document is disapproved if any chunk is disapproved. |
| `providers` | Lists the registered AI backends. `--detect` reports which local server `auto` would use. |

Save and commit flags (on `extract` and `review`): `-o FILE`, `-c/--commit`, `--message`, `--git-init`.

```sh
# Auto-detect (the default): checks --endpoint if given, otherwise Ollama on :11434,
# then OpenAI-compatible servers on :8080, :1234 and :8000. Without -m it uses the first model the server lists.
disapprover review contract.pdf
disapprover providers --detect

# Ollama
disapprover review contract.pdf -p ollama -m llama3.1

# Any OpenAI-compatible server (LocalAI, LM Studio, llama.cpp server, vLLM)
disapprover review contract.pdf -p openai -e http://localhost:1234/v1 -m qwen2.5-7b-instruct

# Strata (OpenAI-compatible on 127.0.0.1:8080, accepts any model name).
# Low reasoning effort is much faster than Strata's default of high.
disapprover review contract.pdf -p openai -e http://127.0.0.1:8080/v1 -m qwen --reasoning-effort low

# Any program: the prompt goes in on stdin and the reply comes out on stdout
disapprover review contract.pdf -p exec --exec "python my_model.py"

# Paths with spaces: --exec is the program as given, and each --exec-arg is passed unchanged
disapprover review contract.pdf -p exec --exec "C:\Program Files\llama\llama-cli.exe" --exec-arg -m --exec-arg "D:\my models\qwen.gguf"

# Custom rules, save the report, commit it, and fail CI on a disapprove verdict
disapprover review contract.pdf -m llama3.1 -r rules.txt -o reviews/contract.json --commit --fail-on-disapprove
```

You can also set the global flags with environment variables: `DISAPPROVER_PROVIDER`, `DISAPPROVER_ENDPOINT`, `DISAPPROVER_MODEL`, `DISAPPROVER_API_KEY`, `DISAPPROVER_EXEC` and `DISAPPROVER_REASONING_EFFORT`.

`--reasoning-effort` (none, low, medium or high) is sent as `reasoning_effort` to OpenAI-compatible servers that support it. If it isn't set, the server's default applies. Any `</think>` text that leaks into a reply is removed before the JSON is read.

Exit codes: `0` means OK, `1` means an error, and `2` means disapproved (only with `--fail-on-disapprove`).

## Rules, laws and approved examples

The model judges the document against local UTF-8 text files. Each flag can be repeated.

| Flag | File contains | Effect |
|---|---|---|
| `-r, --rules` | House rules | Content that breaks a rule is disapproved |
| `--laws` | Text of laws or regulations | Content that is against a law is disapproved, and the finding cites the law and article |
| `--pass-examples` | Content that should pass | Calibrates the model so similar content is not disapproved |
| `--reason-format` | A one-line template | How each finding's reason is written |

If you give no `--rules` and no `--laws`, built-in rules are used (personal data, confidential markings, hate, sexual content, illegal instructions).

Every finding has `page`, `rule_id`, `law`, `article`, `category`, `severity`, `excerpt` and `reason`, plus `message`, which is the finding rendered with the template. The template placeholders are `{page} {rule_id} {law} {article} {citation} {category} {severity} {excerpt} {reason}`. `{citation}` is "law article", or the `rule_id` when there is no law. The default template is `p.{page} [{severity}] {citation}: {reason}`. If the model disapproves without giving a finding, a finding marked "unspecified" is added so a disapproval always shows a reason.

```sh
# reason.txt:  違反{law}{article}：{reason}（第{page}頁）
disapprover review ad.pdf --laws pdpa.txt --laws fair-trade.txt --pass-examples ok-ads.txt --reason-format reason.txt -o reviews/ad.json --commit
```

The report's `policy` field lists each file used, with its SHA-256, so a committed report shows exactly which version of each law file produced the verdict. The policy is sent again with every chunk; if it is over 24,000 characters, the CLI warns you to check that it fits the model's context window.

## Plugging in your own AI

There are three ways to do it:

1. **No code:** use `-p exec --exec "<command>"`.
2. **Register a provider** so that `--provider` can select it:
   ```go
   func init() {
       ai.Register("mybackend", func(cfg ai.Config) (ai.Client, error) { return &My{cfg}, nil })
   }
   ```
3. **Inject a client directly** from your own `main`:
   ```go
   cmd.Execute(cmd.WithClient(ai.ClientFunc(func(ctx context.Context, r ai.Request) (string, error) {
       return myModel.Generate(ctx, r.Messages)
   })))
   ```

The model has to reply with JSON in this shape: `{"verdict":"approve|disapprove","findings":[{"page","rule_id","law","article","category","severity","excerpt","reason"}]}`. The parser ignores code fences and extra text around the JSON.

## Layout

```
main.go
cmd/                 Cobra commands (root, extract, review, providers, save/commit flags)
pkg/ai/              Client interface, registry, auto / ollama / openai / exec providers
internal/pdftext/    PDF loading and per-page text extraction
internal/review/     policy (rules, laws, examples, reason format), chunking, prompt, verdict parsing
internal/store/      writing files and git commits
```

## Limitations

- Scanned PDFs that contain only images have no text to extract. Run OCR on them first.
- Without `--exec-arg`, `--exec` is split on whitespace unless it names an existing file. Use `--exec-arg` for arguments with spaces; shell features such as pipes need a script.

# disapprover

[English](README.md) | 繁體中文

這是一個用 Cobra 寫的 Go 命令列工具：載入 PDF，交給**本地 AI** 依你的規則審查內容，回傳**核准（approve）**或**駁回（disapprove）**。報告可以存成檔案並 commit 到 git。

## 安裝

需要 Go 1.26 以上；使用 `--commit` 時，`PATH` 中也需要有 git。

```sh
go build -o disapprover .        # Windows 上為 disapprover.exe
```

## 功能

| 功能 | 說明 |
|---|---|
| 載入 PDF | 擷取每一頁的文字，並記錄檔案的 SHA-256 |
| AI 內容審查 | 產生核准或駁回的結果，每筆發現都含頁碼、類別、嚴重程度、原文引用與理由 |
| 支援任何本地 AI | Ollama 風格、OpenAI 相容（LocalAI、LM Studio、llama.cpp、vLLM）、任何命令列程式，或在 Go 中注入用戶端 |
| 自訂規則 | 純文字規則檔；未指定時使用內建規則 |
| 大型文件 | 以整頁為單位分段送出（預設每段 8,000 字元）；任一段被駁回，整份文件即被駁回 |
| 儲存並 commit | 一個指令即可寫入檔案，並只對該檔案執行 git commit |
| 適合 CI | `--fail-on-disapprove` 會以狀態碼 2 結束 |

## 指令

| 指令 | 用途 |
|---|---|
| `review <pdf>` | 請 AI 判定 PDF 應核准或駁回，並印出摘要 |
| `extract <pdf>` | 印出 PDF 文字，或用 `-o` 存檔；`--json` 輸出逐頁文字與 SHA-256 |
| `providers` | 列出已註冊的 AI 後端 |

`extract` 與 `review` 共用的儲存參數：`-o FILE`、`-c/--commit`、`--message`、`--git-init`。

`review` 專用參數：`-r/--rules FILE`、`--max-chars N`、`--fail-on-disapprove`、`-q/--quiet`。

## 連接本地 AI

用 `-p` 指定伺服器的 API 風格，預設為 `ollama`。

| Provider | 適用對象 | 預設端點 |
|---|---|---|
| `ollama` | Ollama（`/api/chat`） | `http://localhost:11434` |
| `openai` | LocalAI、LM Studio、llama.cpp server、vLLM、Ollama 的 `/v1` | `http://localhost:8080/v1` |
| `exec` | 任何程式：提示詞從 stdin 傳入，回覆從 stdout 讀取 | 無 |

```sh
# Ollama 風格
disapprover review contract.pdf -m llama3.1

# OpenAI 相容：端點為以 /v1 結尾的基本 URL
disapprover review contract.pdf -p openai -e http://localhost:1234/v1 -m qwen2.5-7b-instruct

# 任何程式
disapprover review contract.pdf -p exec --exec "python my_model.py"

# 自訂規則、儲存報告、commit，並在駁回時讓 CI 失敗
disapprover review contract.pdf -m llama3.1 -r rules.txt -o reviews/contract.json --commit --fail-on-disapprove
```

全域參數也可以用環境變數設定：`DISAPPROVER_PROVIDER`、`DISAPPROVER_ENDPOINT`、`DISAPPROVER_MODEL`、`DISAPPROVER_API_KEY`、`DISAPPROVER_EXEC`。`--timeout` 預設每次 AI 請求 5 分鐘。

結束狀態碼：`0` 成功，`1` 錯誤，`2` 駁回（僅在加上 `--fail-on-disapprove` 時）。

## 規則與報告

規則檔是純文字，寫法就像在向人工審查員說明標準，指定後會完全取代內建規則。

```text
若文件包含以下任一內容，請駁回：
- 客戶姓名與帳號同時出現
- 標註「內部」的報價
否則核准。
```

JSON 報告欄位：`file`、`sha256`、`pages`、`provider`、`model`、`verdict`、`findings[]`（`page`、`category`、`severity`、`excerpt`、`reason`）、`rules`、`reviewed_at`。

## 接入自己的 AI

有三種方式：

1. **不寫程式**：使用 `-p exec --exec "<command>"`。
2. **註冊 provider**，讓 `--provider` 可以選用：
   ```go
   func init() {
       ai.Register("mybackend", func(cfg ai.Config) (ai.Client, error) { return &My{cfg}, nil })
   }
   ```
3. **在自己的 `main` 中直接注入用戶端**：
   ```go
   cmd.Execute(cmd.WithClient(ai.ClientFunc(func(ctx context.Context, r ai.Request) (string, error) {
       return myModel.Generate(ctx, r.Messages)
   })))
   ```

模型必須回覆以下格式的 JSON：`{"verdict":"approve|disapprove","findings":[{"page","category","severity","excerpt","reason"}]}`。JSON 前後的 code fence 或多餘文字會被忽略。

## 專案結構

```
main.go
cmd/                 Cobra 指令（root、extract、review、providers、儲存與 commit 參數）
pkg/ai/              Client 介面、註冊表，以及 ollama / openai / exec provider
internal/pdftext/    PDF 載入與逐頁文字擷取
internal/review/     分段、提示詞、判定結果解析與合併
internal/store/      寫入檔案與 git commit
```

## 疑難排解

| 錯誤訊息包含 | 解法 |
|---|---|
| `--model is required` | 加上 `-m <model>` 或設定 `DISAPPROVER_MODEL` |
| `connection refused` | 啟動伺服器；檢查 `-e` 與 `-p` 是否正確 |
| `404 Not Found` | Provider 與伺服器風格不符：Ollama 用 `-p ollama`，LocalAI、LM Studio 用 `-p openai` |
| `has no extractable text` | 掃描型 PDF，請先做 OCR（例如 `ocrmypdf`） |
| `model reply has no JSON object` | 換用更能遵循指示的模型，或調低 `--max-chars` |
| `not inside a git repository` | 加上 `--git-init`，或存到儲存庫內 |
| `Please tell me who you are` | 設定 `git config --global user.name` 與 `user.email` |

## 限制

- 只含圖片的掃描 PDF 無法擷取文字，請先做 OCR。
- `--exec` 以空白切割指令，複雜的指令請包成腳本。

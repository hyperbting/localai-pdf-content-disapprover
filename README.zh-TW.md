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
| 規則與法規 | 純文字的規則檔、法規檔與核准範例；違法即駁回，理由會引用法規與條號，格式可用範本自訂 |
| 大型文件 | 以整頁為單位分段送出（預設每段 8,000 字元）；任一段被駁回，整份文件即被駁回 |
| 儲存並 commit | 一個指令即可寫入檔案，並只對該檔案執行 git commit |
| 適合 CI | `--fail-on-disapprove` 會以狀態碼 2 結束 |

## 指令

| 指令 | 用途 |
|---|---|
| `review <pdf>` | 請 AI 判定 PDF 應核准或駁回，並印出摘要 |
| `extract <pdf>` | 印出 PDF 文字，或用 `-o` 存檔；`--json` 輸出逐頁文字與 SHA-256 |
| `providers` | 列出已註冊的 AI 後端；`--detect` 顯示 `auto` 會使用哪個本地伺服器 |

`extract` 與 `review` 共用的儲存參數：`-o FILE`、`-c/--commit`、`--message`、`--git-init`。

`review` 專用參數：`-r/--rules`、`--laws`、`--pass-examples`、`--reason-format`、`--max-chars N`、`--retries N`、`--fail-on-disapprove`、`-q/--quiet`。

## 連接本地 AI

用 `-p` 指定伺服器的 API 風格，預設為 `auto`（自動偵測）。

| Provider | 適用對象 | 預設端點 |
|---|---|---|
| `auto` | 自動偵測：有 `--endpoint` 就檢查它，否則依序檢查 Ollama :11434，再檢查 OpenAI 相容的 :8080、:1234、:8000；未指定 `-m` 時使用伺服器列出的第一個模型 | 依偵測結果 |
| `ollama` | Ollama（`/api/chat`） | `http://localhost:11434` |
| `openai` | LocalAI、LM Studio、llama.cpp server、vLLM、Strata、Ollama 的 `/v1` | `http://localhost:8080/v1` |
| `exec` | 任何程式：提示詞從 stdin 傳入，回覆從 stdout 讀取 | 無 |

```sh
# 自動偵測（預設）
disapprover review contract.pdf
disapprover providers --detect

# Ollama 風格
disapprover review contract.pdf -p ollama -m llama3.1

# OpenAI 相容：端點為以 /v1 結尾的基本 URL
disapprover review contract.pdf -p openai -e http://localhost:1234/v1 -m qwen2.5-7b-instruct

# Strata（OpenAI 相容，位於 127.0.0.1:8080，任何模型名稱皆可）
# 低推理強度比 Strata 預設的 high 快很多
disapprover review contract.pdf -p openai -e http://127.0.0.1:8080/v1 -m qwen --reasoning-effort low

# 任何程式
disapprover review contract.pdf -p exec --exec "python my_model.py"

# 路徑含空白：--exec 視為完整程式路徑，每個 --exec-arg 原樣傳入
disapprover review contract.pdf -p exec --exec "C:\Program Files\llama\llama-cli.exe" --exec-arg -m --exec-arg "D:\my models\qwen.gguf"

# 自訂規則、儲存報告、commit，並在駁回時讓 CI 失敗
disapprover review contract.pdf -m llama3.1 -r rules.txt -o reviews/contract.json --commit --fail-on-disapprove
```

全域參數也可以用環境變數設定：`DISAPPROVER_PROVIDER`、`DISAPPROVER_ENDPOINT`、`DISAPPROVER_MODEL`、`DISAPPROVER_API_KEY`、`DISAPPROVER_EXEC`、`DISAPPROVER_REASONING_EFFORT`。

`--reasoning-effort`（none、low、medium、high）會以 `reasoning_effort` 送給支援它的 OpenAI 相容伺服器；未設定時沿用伺服器預設值。回覆中若混入 `</think>` 思考內容，會在解析 JSON 前移除。`--timeout` 預設每次 AI 請求 5 分鐘。

結束狀態碼：`0` 成功，`1` 錯誤，`2` 駁回（僅在加上 `--fail-on-disapprove` 時）。

## 規則、法規與核准範例

模型會依據本地 UTF-8 文字檔審查文件，每個參數都可重複指定。

| 參數 | 檔案內容 | 作用 |
|---|---|---|
| `-r, --rules` | 內部規則 | 違反規則的內容會被駁回 |
| `--laws` | 法律或法規條文 | 違法的內容會被駁回，發現項目會引用法規名稱與條號 |
| `--pass-examples` | 應通過的內容範例 | 校正模型，避免把類似內容誤判為駁回 |
| `--reason-format` | 單行範本 | 決定每筆駁回理由的寫法 |

未指定 `--rules` 與 `--laws` 時，使用內建規則（個資、機密標註、仇恨言論、色情內容、違法指引）。

規則檔是純文字，寫法就像在向人工審查員說明標準：

```text
若文件包含以下任一內容，請駁回：
- 客戶姓名與帳號同時出現
- 標註「內部」的報價
否則核准。
```

每筆發現都包含 `page`、`rule_id`、`law`、`article`、`category`、`severity`、`excerpt`、`reason`，以及依範本產生的 `message`。範本可用的佔位符：`{page} {rule_id} {law} {article} {citation} {category} {severity} {excerpt} {reason}`；`{citation}` 為「法規 條號」，沒有法規時改用 `rule_id`。預設範本為 `p.{page} [{severity}] {citation}: {reason}`。若模型駁回卻沒有給出發現項目，會補上一筆標為「unspecified」的發現，確保駁回一定附有理由。

```sh
# reason.txt：違反{law}{article}：{reason}（第{page}頁）
disapprover review ad.pdf --laws 個資法.txt --laws 公平交易法.txt --pass-examples 合格廣告.txt --reason-format reason.txt -o reviews/ad.json --commit
```

```text
DISAPPROVED  ad.pdf  (2 pages, 1 findings)
  - 違反個人資料保護法第6條：含有病歷資料（第2頁）
      "糖尿病病史"
```

JSON 報告欄位：`file`、`sha256`、`pages`、`provider`、`model`、`verdict`、`findings[]`、`policy`、`reviewed_at`。`policy` 會列出每個使用的檔案及其 SHA-256，commit 後即可追溯是哪個版本的法規檔產生了這個判定。政策內容會隨每個分段重送；超過 24,000 字元時會提醒你確認模型的 context 是否容得下。

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

模型必須回覆以下格式的 JSON：`{"verdict":"approve|disapprove","findings":[{"page","rule_id","law","article","category","severity","excerpt","reason"}]}`。JSON 前後的 code fence 或多餘文字會被忽略。若回覆仍無法解析，會把該回覆與錯誤訊息送回模型並重問；次數由 `--retries N` 設定（預設 1，`0` 為關閉）。

## 專案結構

```
main.go
cmd/                 Cobra 指令（root、extract、review、providers、儲存與 commit 參數）
pkg/ai/              Client 介面、註冊表，以及 auto / ollama / openai / exec provider
internal/pdftext/    PDF 載入與逐頁文字擷取
internal/review/     政策（規則、法規、範例、理由格式）、分段、提示詞、判定結果解析
internal/store/      寫入檔案與 git commit
```

## 疑難排解

| 錯誤訊息包含 | 解法 |
|---|---|
| `--model is required` | 加上 `-m <model>` 或設定 `DISAPPROVER_MODEL` |
| `connection refused` | 啟動伺服器；檢查 `-e` 與 `-p` 是否正確 |
| `404 Not Found` | Provider 與伺服器風格不符：Ollama 用 `-p ollama`，LocalAI、LM Studio 用 `-p openai` |
| `has no extractable text` | 掃描型 PDF，請先做 OCR（例如 `ocrmypdf`） |
| `model reply has no JSON object` | 預設會帶著錯誤訊息重問一次；可提高 `--retries`、換用更能遵循指示的模型，或調低 `--max-chars` |
| `not inside a git repository` | 加上 `--git-init`，或存到儲存庫內 |
| `Please tell me who you are` | 設定 `git config --global user.name` 與 `user.email` |

## 限制

- 只含圖片的掃描 PDF 無法擷取文字，請先做 OCR。
- 未使用 `--exec-arg` 時，`--exec` 會以空白切割，除非它指向一個存在的檔案。含空白的參數請用 `--exec-arg`；管線等 shell 功能請包成腳本。

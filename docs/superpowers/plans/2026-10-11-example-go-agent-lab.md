# example-go-agent-lab 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建四步递进的 Go 智能体 demo：MCP Server → MCP Client+LLM 工具循环 → RAG → 综合智能体（RAG 工具化）。

**Architecture:** 单 module 多 cmd。`internal/` 沉淀可复用组件：provider 注册表（三家 OpenAI 兼容端点）、llm 封装（openai-go + BaseURL 切换）、expr 表达式求值、mcp2llm 格式转换、agent 工具循环、splitter 文本切分。四个 `cmd/` demo 逐层组合这些组件。MCP 走 stdio 子进程；向量库用 chromem-go 嵌入式持久化。

**Tech Stack:** Go 1.26.4 · github.com/mark3labs/mcp-go v1.2.1 · github.com/openai/openai-go/v3 v3.76.0 · github.com/philippgille/chromem-go v0.7.0 · gopkg.in/yaml.v3 v3.0.1

**Spec:** `docs/superpowers/specs/2026-10-11-go-agent-lab-design.md`

## Global Constraints

- module 名 `example-go-agent-lab`，Go 1.26.4（go.mod 已就位）
- 依赖版本固定：mcp-go v1.2.1 / openai-go v3 v3.76.0 / chromem-go v0.7.0 / yaml.v3 v3.0.1
- **mcp-go 类型导入路径必须是 `github.com/mark3labs/mcp-go/mcp`**（v1.x 根目录没有 Go 包；server/client 分别在 `/server`、`/client` 子包）
- openai-go 导入：`openai "github.com/openai/openai-go/v3"`、`"github.com/openai/openai-go/v3/option"`、`"github.com/openai/openai-go/v3/shared"`
- API key 只从环境变量读取（`DEEPSEEK_API_KEY`/`ZHIPU_API_KEY`/`DASHSCOPE_API_KEY`/`SILICONFLOW_API_KEY`），任何提交文件不得含真实 key
- 代码注释、README、docs 一律中文
- 不引入 langchaingo；不做流式/Web UI/独立向量库/多 Agent（spec 非目标）
- 单元测试不得访问真实 API——只测纯函数与注入的 fake
- 运行环境 Windows 11 + Git Bash；所有 demo 从项目根目录 `go run ./cmd/...`
- 每个任务结束 `go build ./... && go vet ./...` 通过后 git commit（中文提交信息）

## 文件结构总览

```
example-go-agent-lab/
├── README.md                       # Task 9
├── .gitignore                      # Task 1
├── configs/config.yaml             # Task 1
├── internal/
│   ├── appenv/appenv.go            # Task 1  环境变量读取
│   ├── provider/provider.go        # Task 1  端点注册表
│   ├── config/config.go            # Task 1  yaml 配置加载
│   ├── llm/llm.go                  # Task 2  OpenAI 兼容客户端封装
│   ├── expr/expr.go                # Task 3  安全表达式求值（demo01 用）
│   ├── mcp2llm/convert.go          # Task 4  MCP schema → function calling
│   ├── agent/agent.go              # Task 5  工具循环（可注入 fake）
│   └── splitter/splitter.go        # Task 7  文本切分（demo03 用）
├── cmd/
│   ├── 01-mcp-server/main.go       # Task 3
│   ├── 02-mcp-agent/main.go        # Task 6
│   ├── 03-rag/main.go              # Task 8
│   └── 04-combo-agent/main.go      # Task 9
└── docs/
    ├── notes/                      # Task 3  4 篇中文笔记（demo01/03/04 语料）
    ├── mcp-server.md               # Task 9  四篇原理说明
    ├── mcp-agent.md
    ├── rag.md
    └── combo-agent.md
```

已验证的关键 API 事实（执行者直接引用，勿凭记忆改写）：

```go
// mcp-go v1.2.1
server.NewMCPServer(name, version string, opts ...server.ServerOption) *server.MCPServer
s.AddTool(tool mcp.Tool, handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error))
server.ServeStdio(s) error
mcp.NewTool(name string, opts ...mcp.ToolOption) mcp.Tool            // ToolOption: mcp.WithDescription/WithString/WithNumber
                                                                      // PropertyOption: mcp.Required()/mcp.Description()/mcp.Enum(...)
req.RequireString(key) (string, error) / req.GetString(key, def)
mcp.NewToolResultText(s) / mcp.NewToolResultErrorf(format, ...) / mcp.NewToolResultErrorFromErr(s, err)
client.NewStdioMCPClient(command string, env []string, args ...string) (*client.Client, error)
c.Start(ctx) error / c.Initialize(ctx, mcp.InitializeRequest) / c.ListTools(ctx, mcp.ListToolsRequest{}) / c.CallTool(ctx, mcp.CallToolRequest)
c.Close() error / client.GetStderr(c) (io.Reader, bool)
mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name, Version}}}
mcp.CallToolRequest{Params: mcp.CallToolParams{Name: string, Arguments: any}}   // Arguments 传 map[string]any
mcp.CallToolResult{Content []mcp.Content, IsError bool}                          // 文本: content.(mcp.TextContent).Text
mcp.Tool{Name, Description string, InputSchema mcp.ToolInputSchema}
mcp.ToolInputSchema{Type string, Properties map[string]any, Required []string}

// openai-go v3.76.0
openai.NewClient(option.WithAPIKey(k), option.WithBaseURL(u)) openai.Client   // 值类型
c.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{Model: openai.ChatModel(s), Messages, Tools})
c.Embeddings.New(ctx, openai.EmbeddingNewParams{Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(s)}, Model: openai.EmbeddingModel(s)})
openai.SystemMessage(s)/UserMessage(s)/ToolMessage(content, toolCallID) → openai.ChatCompletionMessageParamUnion
resp.Choices[0].FinishReason string("stop"/"tool_calls") / .Message.Content string
resp.Choices[0].Message.ToolCalls → []openai.ChatCompletionMessageToolCallUnion{ID, Type string, Function struct{Name, Arguments string}}
openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name, Description param.Opt[string], Parameters shared.FunctionParameters}) → tool union
shared.FunctionParameters = map[string]any；param.Opt 的取值字段为 .Value（导出）
embedding 响应: resp.Data[i].Embedding []float64

// chromem-go v0.7.0
chromem.NewPersistentDB(path string, compress bool) (*chromem.DB, error)
db.GetOrCreateCollection(name string, metadata map[string]string, ef chromem.EmbeddingFunc) (*chromem.Collection, error)
chromem.EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)
chromem.Document{ID, Content string, Metadata map[string]string}
col.AddDocuments(ctx, []chromem.Document, concurrency int) error
col.Query(ctx, queryText string, nResults int, where, whereDocument map[string]string) ([]chromem.Result, error)
chromem.Result{ID, Content string, Metadata map[string]string, Similarity float32}
```

---

### Task 1: 脚手架 + provider 注册表 + 配置加载

**Files:**
- Delete: `probe/`（API 验证用的临时目录）
- Create: `.gitignore`
- Create: `configs/config.yaml`
- Create: `internal/appenv/appenv.go`
- Create: `internal/appenv/appenv_test.go`
- Create: `internal/provider/provider.go`
- Create: `internal/provider/provider_test.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Consumes: 无（首个任务）
- Produces:
  - `appenv.Lookup(envName string) (string, error)` — 缺失时返回指明环境变量名的错误
  - `provider.Endpoint struct{Name, BaseURL, APIKeyEnv, DefaultChatModel, DefaultEmbeddingModel string}`
  - `provider.Chat(name string) (Endpoint, error)` — name ∈ deepseek|glm|qwen
  - `provider.Embedding(name string) (Endpoint, error)` — name ∈ qwen|glm|siliconflow；deepseek 报"无 embedding 接口"
  - `config.Config struct{Provider, EmbeddingProvider, ChatModel, EmbeddingModel string}`
  - `config.Load(path string) (Config, error)` — Provider 空默认 "deepseek"，EmbeddingProvider 空默认 "qwen"

- [ ] **Step 1: 清理探针目录 + .gitignore**

```bash
rm -rf probe/
go mod tidy
```

创建 `.gitignore`（若已存在则确保包含以下三行）：

```
.superpowers/
.chromem/
*.exe
```

- [ ] **Step 2: 写 appenv 失败测试**

`internal/appenv/appenv_test.go`：

```go
package appenv

import (
	"strings"
	"testing"
)

func TestLookupFound(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	v, err := Lookup("DEEPSEEK_API_KEY")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if v != "sk-test" {
		t.Fatalf("got %q, want %q", v, "sk-test")
	}
}

func TestLookupMissing(t *testing.T) {
	_, err := Lookup("SOME_MISSING_KEY_XYZ")
	if err == nil {
		t.Fatal("缺失时应返回错误")
	}
	if !strings.Contains(err.Error(), "SOME_MISSING_KEY_XYZ") {
		t.Fatalf("错误信息应包含环境变量名，got: %v", err)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/appenv/`
Expected: FAIL（`undefined: Lookup`，包不存在）

- [ ] **Step 4: 实现 appenv**

`internal/appenv/appenv.go`：

```go
// Package appenv 提供环境变量读取：API key 一律从环境变量来，绝不落盘。
package appenv

import "fmt"

// Lookup 读取环境变量；缺失时返回指明变量名的错误，调用方 log.Fatal 即可。
func Lookup(envName string) (string, error) {
	v := os.Getenv(envName)
	if v == "" {
		return "", fmt.Errorf("缺少环境变量 %s（请先设置，例如: export %s=sk-xxx）", envName, envName)
	}
	return v, nil
}
```

补上 `import "os"`（上面省略仅为紧凑——实现时写全 import）。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./internal/appenv/`
Expected: PASS（ok example-go-agent-lab/internal/appenv）

- [ ] **Step 6: 写 provider 失败测试**

`internal/provider/provider_test.go`：

```go
package provider

import "testing"

func TestChatKnownProviders(t *testing.T) {
	cases := []struct{ name, baseURL, keyEnv, model string }{
		{"deepseek", "https://api.deepseek.com", "DEEPSEEK_API_KEY", "deepseek-chat"},
		{"glm", "https://open.bigmodel.cn/api/paas/v4", "ZHIPU_API_KEY", "glm-4.6"},
		{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", "DASHSCOPE_API_KEY", "qwen-plus"},
	}
	for _, c := range cases {
		ep, err := Chat(c.name)
		if err != nil {
			t.Fatalf("Chat(%q) 不应报错: %v", c.name, err)
		}
		if ep.BaseURL != c.baseURL || ep.APIKeyEnv != c.keyEnv || ep.DefaultChatModel != c.model {
			t.Errorf("Chat(%q) = %+v, 不符合预期", c.name, ep)
		}
	}
}

func TestChatUnknown(t *testing.T) {
	if _, err := Chat("openai"); err == nil {
		t.Fatal("未知 provider 应报错")
	}
}

func TestEmbeddingKnown(t *testing.T) {
	cases := []struct{ name, model string }{
		{"qwen", "text-embedding-v4"},
		{"glm", "embedding-3"},
		{"siliconflow", "BAAI/bge-m3"},
	}
	for _, c := range cases {
		ep, err := Embedding(c.name)
		if err != nil {
			t.Fatalf("Embedding(%q) 不应报错: %v", c.name, err)
		}
		if ep.DefaultEmbeddingModel != c.model {
			t.Errorf("Embedding(%q).DefaultEmbeddingModel = %q, want %q", c.name, ep.DefaultEmbeddingModel, c.model)
		}
	}
}

func TestEmbeddingDeepSeekRejected(t *testing.T) {
	if _, err := Embedding("deepseek"); err == nil {
		t.Fatal("deepseek 无 embedding 接口，应报错")
	}
}
```

- [ ] **Step 7: 运行确认失败**

Run: `go test ./internal/provider/`
Expected: FAIL（`undefined: Chat`）

- [ ] **Step 8: 实现 provider**

`internal/provider/provider.go`：

```go
// Package provider 维护"OpenAI 兼容"端点注册表：
// 三家聊天（deepseek/glm/qwen）+ 三家 embedding（qwen/glm/siliconflow）。
// 模型迭代快，默认模型名集中在这里，config.yaml 可覆盖。
package provider

import "fmt"

// Endpoint 描述一个 OpenAI 兼容服务端点。
type Endpoint struct {
	Name                  string // 注册名，如 deepseek
	BaseURL               string // OpenAI 兼容 API 根地址
	APIKeyEnv             string // 存放 API key 的环境变量名
	DefaultChatModel      string // 默认聊天模型（空 = 不提供聊天）
	DefaultEmbeddingModel string // 默认 embedding 模型（空 = 不提供 embedding）
}

var chatEndpoints = map[string]Endpoint{
	"deepseek": {Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKeyEnv: "DEEPSEEK_API_KEY", DefaultChatModel: "deepseek-chat"},
	"glm":      {Name: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIKeyEnv: "ZHIPU_API_KEY", DefaultChatModel: "glm-4.6", DefaultEmbeddingModel: "embedding-3"},
	"qwen":     {Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKeyEnv: "DASHSCOPE_API_KEY", DefaultChatModel: "qwen-plus", DefaultEmbeddingModel: "text-embedding-v4"},
}

var embeddingEndpoints = map[string]Endpoint{
	"qwen":         chatEndpoints["qwen"],
	"glm":          chatEndpoints["glm"],
	"siliconflow":  {Name: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1", APIKeyEnv: "SILICONFLOW_API_KEY", DefaultEmbeddingModel: "BAAI/bge-m3"},
}

// Chat 返回指定聊天 provider 的端点信息。
func Chat(name string) (Endpoint, error) {
	ep, ok := chatEndpoints[name]
	if !ok {
		return Endpoint{}, fmt.Errorf("未知聊天 provider %q（可选: deepseek | glm | qwen）", name)
	}
	return ep, nil
}

// Embedding 返回指定 embedding provider 的端点信息。
// deepseek 不提供 /embeddings 接口，聊天用 deepseek 时 embedding 需另选一家。
func Embedding(name string) (Endpoint, error) {
	ep, ok := embeddingEndpoints[name]
	if !ok {
		return Endpoint{}, fmt.Errorf("未知 embedding provider %q（可选: qwen | glm | siliconflow；deepseek 无 embedding 接口）", name)
	}
	return ep, nil
}
```

- [ ] **Step 9: 运行确认通过**

Run: `go test ./internal/provider/`
Expected: PASS

- [ ] **Step 10: 写 config 失败测试**

`internal/config/config_test.go`：

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadExplicit(t *testing.T) {
	path := writeTemp(t, "provider: glm\nembedding_provider: siliconflow\nchat_model: glm-4-flash\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "glm" || cfg.EmbeddingProvider != "siliconflow" || cfg.ChatModel != "glm-4-flash" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	path := writeTemp(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "deepseek" {
		t.Errorf("默认 provider 应为 deepseek，got %q", cfg.Provider)
	}
	if cfg.EmbeddingProvider != "qwen" {
		t.Errorf("默认 embedding_provider 应为 qwen，got %q", cfg.EmbeddingProvider)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("no/such/file.yaml"); err == nil {
		t.Fatal("文件不存在应报错")
	}
}
```

- [ ] **Step 11: 运行确认失败**

Run: `go test ./internal/config/`
Expected: FAIL（`undefined: Load`）

- [ ] **Step 12: 实现 config + configs/config.yaml**

`internal/config/config.go`：

```go
// Package config 读取 configs/config.yaml。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 是全项目共享的运行配置。
// ChatModel/EmbeddingModel 为空时使用 provider 注册表里的默认模型（可覆盖，防模型名过时）。
type Config struct {
	Provider          string `yaml:"provider"`           // deepseek | glm | qwen
	EmbeddingProvider string `yaml:"embedding_provider"` // qwen | glm | siliconflow
	ChatModel         string `yaml:"chat_model"`         // 可选：覆盖聊天模型名
	EmbeddingModel    string `yaml:"embedding_model"`    // 可选：覆盖 embedding 模型名
}

// Load 从 yaml 文件读取配置并填充默认值。
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	if cfg.Provider == "" {
		cfg.Provider = "deepseek"
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = "qwen"
	}
	return cfg, nil
}
```

`configs/config.yaml`：

```yaml
# 聊天 provider：deepseek | glm | qwen
# DeepSeek 没有 /embeddings 接口，所以 embedding 单独配一家
provider: deepseek
embedding_provider: qwen

# 可选：覆盖默认模型名（默认见 internal/provider/provider.go）
# chat_model: deepseek-chat
# embedding_model: text-embedding-v4
```

- [ ] **Step 13: 全量测试 + 提交**

```bash
go mod tidy && go build ./... && go vet ./... && go test ./...
git add .gitignore configs/ internal/ go.mod go.sum
git commit -m "feat: 项目脚手架——provider 注册表、yaml 配置、环境变量读取"
```

Expected: `ok example-go-agent-lab/internal/appenv`、`internal/config`、`internal/provider` 全绿

---

### Task 2: internal/llm — OpenAI 兼容客户端封装

**Files:**
- Create: `internal/llm/llm.go`
- Create: `internal/llm/llm_test.go`

**Interfaces:**
- Consumes: 无 SDK 之外的前置依赖（Task 1 的包本任务不用）
- Produces:
  - `type Message = openai.ChatCompletionMessageParamUnion`（类型别名）
  - `type ToolDef = openai.ChatCompletionToolUnionParam`（类型别名）
  - `type ToolCall struct{ ID, Name, Arguments string }`
  - `func AssistantWithToolCalls(content string, calls []ToolCall) Message`
  - `func ToolResult(content, callID string) Message`
  - `func New(baseURL, apiKey, model string) *Client`
  - `func (c *Client) Complete(ctx, msgs []Message, tools []ToolDef) (string, []ToolCall, error)` — 返回回答文本与解析好的工具调用
  - `func NewEmbedder(baseURL, apiKey, model string) *Embedder`
  - `func (e *Embedder) Embed(ctx, texts []string) ([][]float32, error)` — 批量向量化，float64→float32

说明：Complete/Embed 是对 SDK 的透传（无本地逻辑可单测），单测覆盖的是消息构造 helper 的 JSON 形状；这两个方法的真实连通性由 demo 02/03 的运行验收覆盖。

- [ ] **Step 1: 写 helper 失败测试**

`internal/llm/llm_test.go`：

```go
package llm

import (
	"encoding/json"
	"testing"
)

// 经 JSON 断言而非直接读 union 字段：序列化形状才是发给 API 的最终事实。
func TestAssistantWithToolCalls(t *testing.T) {
	m := AssistantWithToolCalls("", []ToolCall{
		{ID: "call_1", Name: "calculate", Arguments: `{"expression":"1+2"}`},
	})
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["role"] != "assistant" {
		t.Errorf("role = %v, want assistant", got["role"])
	}
	calls, ok := got["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %v", got["tool_calls"])
	}
	c0 := calls[0].(map[string]any)
	if c0["id"] != "call_1" || c0["type"] != "function" {
		t.Errorf("tool_call = %v", c0)
	}
	fn := c0["function"].(map[string]any)
	if fn["name"] != "calculate" {
		t.Errorf("function.name = %v", fn["name"])
	}
	if fn["arguments"] != `{"expression":"1+2"}` {
		t.Errorf("function.arguments = %v", fn["arguments"])
	}
}

func TestToolResult(t *testing.T) {
	m := ToolResult("42", "call_9")
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["role"] != "tool" {
		t.Errorf("role = %v, want tool", got["role"])
	}
	if got["tool_call_id"] != "call_9" {
		t.Errorf("tool_call_id = %v", got["tool_call_id"])
	}
	if got["content"] != "42" {
		t.Errorf("content = %v", got["content"])
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/llm/`
Expected: FAIL（`undefined: AssistantWithToolCalls`）

- [ ] **Step 3: 实现 llm 包**

`internal/llm/llm.go`：

```go
// Package llm 封装 OpenAI 兼容的聊天与 embedding 接口。
// 通过 option.WithBaseURL 切换 DeepSeek/GLM/Qwen/硅基流动等兼容服务。
package llm

import (
	"context"
	"fmt"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Message 是发给聊天接口的一条消息（openai-go 参数 union 的别名）。
type Message = openai.ChatCompletionMessageParamUnion

// ToolDef 是传给模型的工具定义（function calling 格式）。
type ToolDef = openai.ChatCompletionToolUnionParam

// ToolCall 是模型返回的一次工具调用请求。
type ToolCall struct {
	ID        string // 工具调用 ID，回填结果时必须带上
	Name      string
	Arguments string // JSON 字符串，由调用方解析
}

// AssistantWithToolCalls 构造"带 tool_calls 的 assistant 消息"，
// 用于把模型的工具调用决定写回对话历史。
func AssistantWithToolCalls(content string, calls []ToolCall) Message {
	tcParams := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, c := range calls {
		tcParams = append(tcParams, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: c.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      c.Name,
					Arguments: c.Arguments,
				},
			},
		})
	}
	return Message{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(content),
			},
			ToolCalls: tcParams,
		},
	}
}

// ToolResult 构造 tool 角色的结果消息，callID 必须与对应的 tool_call 一致。
func ToolResult(content, callID string) Message {
	return openai.ToolMessage(content, callID)
}

// Client 是聊天客户端。
type Client struct {
	sc    openai.Client
	model string
}

// New 创建聊天客户端；baseURL 形如 https://api.deepseek.com。
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		sc: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
		),
		model: model,
	}
}

// Complete 发送对话并返回回答文本与工具调用。
// FinishReason 为 tool_calls 时 calls 非空；模型给出最终回答时 calls 为空。
func (c *Client) Complete(ctx context.Context, msgs []Message, tools []ToolDef) (string, []ToolCall, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: msgs,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	resp, err := c.sc.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", nil, fmt.Errorf("调用聊天接口失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("模型未返回任何候选回复")
	}
	choice := resp.Choices[0]
	var calls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" {
			continue
		}
		calls = append(calls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return choice.Message.Content, calls, nil
}

// Embedder 是向量化客户端。
type Embedder struct {
	sc    openai.Client
	model string
}

// NewEmbedder 创建 embedding 客户端（走 /embeddings 兼容接口）。
func NewEmbedder(baseURL, apiKey, model string) *Embedder {
	return &Embedder{
		sc: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
		),
		model: model,
	}
}

// Embed 批量向量化。chromem-go 需要 []float32，这里完成 float64→float32 转换。
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	resp, err := e.sc.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("调用 embedding 接口失败: %w", err)
	}
	out := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		v := make([]float32, len(d.Embedding))
		for j, f := range d.Embedding {
			v[j] = float32(f)
		}
		out[i] = v
	}
	return out, nil
}
```

- [ ] **Step 4: 运行确认通过 + 提交**

```bash
go mod tidy && go build ./... && go vet ./... && go test ./...
git add internal/llm go.mod go.sum
git commit -m "feat: llm 封装——OpenAI 兼容聊天/Embedding + 消息构造 helper"
```

Expected: 测试全绿（此任务起 openai-go 进入 go.mod require）

---

### Task 3: 表达式求值器 + 笔记语料 + demo 01 MCP Server

**Files:**
- Create: `internal/expr/expr.go`
- Create: `internal/expr/expr_test.go`
- Create: `docs/notes/go-concurrency.md`
- Create: `docs/notes/mcp-protocol.md`
- Create: `docs/notes/rag-basics.md`
- Create: `docs/notes/agent-patterns.md`
- Create: `cmd/01-mcp-server/main.go`

**Interfaces:**
- Consumes: 无前置任务依赖
- Produces:
  - `expr.Evaluate(input string) (float64, error)` — 安全的四则运算求值（手写词法+递归下降，绝非 eval）
  - demo01 可执行：stdio MCP 服务器，工具 `get_time` / `calculate` / `read_note`

- [ ] **Step 1: 写 expr 失败测试**

`internal/expr/expr_test.go`：

```go
package expr

import (
	"math"
	"testing"
)

func TestEvaluateValid(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"1+2*3", 7},
		{"(1+2)*3", 9},
		{"10/4", 2.5},
		{"-(3+4)", -7},
		{"2.5*4", 10},
		{"1 + 2 * (3 - 1) / 2", 3},
		{"0.1+0.2", 0.3},
		{"((2))", 2},
		{"  7  ", 7},
	}
	for _, c := range cases {
		got, err := Evaluate(c.input)
		if err != nil {
			t.Errorf("Evaluate(%q) 不应报错: %v", c.input, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Evaluate(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestEvaluateInvalid(t *testing.T) {
	for _, input := range []string{
		"",      // 空
		"1+",    // 缺右操作数
		"(1+2",  // 缺右括号
		"3$5",   // 非法字符
		"1/0",   // 除零
		"2 3",   // 多余 token
		"1.2.3", // 非法数字
	} {
		if _, err := Evaluate(input); err == nil {
			t.Errorf("Evaluate(%q) 应报错", input)
		}
	}
}
```

（Valid 用例已内联 `math.Abs(got-c.want) > 1e-9` 容差判断，无需额外 helper；`math` 与 `testing` 两个 import 即可。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/expr/`
Expected: FAIL（`undefined: Evaluate`）

- [ ] **Step 3: 实现 expr（两阶段：tokenize → 递归下降）**

`internal/expr/expr.go`：

```go
// Package expr 实现安全的四则运算表达式求值。
// 教学要点：绝不用 eval 类机制——手写词法分析 + 递归下降解析，
// 非法输入只会得到明确报错，不可能执行任意代码。
package expr

import (
	"fmt"
	"strconv"
	"unicode"
)

// Evaluate 计算表达式：支持 + - * /、括号、一元负号、整数与小数。
func Evaluate(input string) (float64, error) {
	toks, err := tokenize(input)
	if err != nil {
		return 0, err
	}
	p := &parser{toks: toks}
	val, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if tail := p.peek(); tail.kind != tokEOF {
		return 0, fmt.Errorf("表达式末尾有多余内容 %q", tail.text)
	}
	return val, nil
}

type tokenKind int

const (
	tokNumber tokenKind = iota
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokLParen
	tokRParen
	tokEOF
)

type token struct {
	kind tokenKind
	text string
}

// tokenize 把源串切成 token 流；跳过空白，拒绝任何非法字符。
func tokenize(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r >= '0' && r <= '9' || r == '.':
			start := i
			dots := 0
			for i < len(rs) {
				c := rs[i]
				if c >= '0' && c <= '9' {
					i++
				} else if c == '.' {
					dots++
					i++
				} else {
					break
				}
			}
			text := string(rs[start:i])
			if dots > 1 {
				return nil, fmt.Errorf("非法数字 %q", text)
			}
			toks = append(toks, token{kind: tokNumber, text: text})
		case r == '+':
			toks = append(toks, token{kind: tokPlus, text: "+"})
			i++
		case r == '-':
			toks = append(toks, token{kind: tokMinus, text: "-"})
			i++
		case r == '*':
			toks = append(toks, token{kind: tokStar, text: "*"})
			i++
		case r == '/':
			toks = append(toks, token{kind: tokSlash, text: "/"})
			i++
		case r == '(':
			toks = append(toks, token{kind: tokLParen, text: "("})
			i++
		case r == ')':
			toks = append(toks, token{kind: tokRParen, text: ")"})
			i++
		default:
			return nil, fmt.Errorf("无法识别的字符 %q", string(r))
		}
	}
	toks = append(toks, token{kind: tokEOF})
	return toks, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{kind: tokEOF}
}

func (p *parser) next() token {
	t := p.peek()
	p.pos++
	return t
}

// 文法（优先级从低到高）：
//   expr   = term (('+'|'-') term)*
//   term   = factor (('*'|'/') factor)*
//   factor = NUMBER | '(' expr ')' | '-' factor

func (p *parser) parseExpr() (float64, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek().kind {
		case tokPlus:
			p.next()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val += r
		case tokMinus:
			p.next()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val -= r
		default:
			return val, nil
		}
	}
}

func (p *parser) parseTerm() (float64, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek().kind {
		case tokStar:
			p.next()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			val *= r
		case tokSlash:
			p.next()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("除数为零")
			}
			val /= r
		default:
			return val, nil
		}
	}
}

func (p *parser) parseFactor() (float64, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return strconv.ParseFloat(t.text, 64)
	case tokLParen:
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.next().kind != tokRParen {
			return 0, fmt.Errorf("缺少右括号")
		}
		return val, nil
	case tokMinus:
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	default:
		return 0, fmt.Errorf("意外的符号 %q", t.text)
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/expr/`
Expected: PASS（全部用例）

- [ ] **Step 5: 写笔记语料（4 篇，demo01 read_note 与 demo03/04 RAG 共用）**

`docs/notes/go-concurrency.md`：

```markdown
# Go 并发编程要点

## goroutine 与 channel

goroutine 是 Go 的轻量级线程，`go f(x)` 一行即可启动。channel 是 goroutine
间的通信管道："不要通过共享内存来通信，而要通过通信来共享内存"。

- 无缓冲 channel：发送阻塞直到有人接收（同步点）
- 带缓冲 channel：`make(chan int, 3)` 缓冲满前发送不阻塞
- 关闭：发送方 `close(ch)`；接收方 `v, ok := <-ch` 中 ok 为 false 表示已关闭

## select 多路复用

`select` 同时等待多个 channel，哪个先就绪就执行哪个分支；带 `default`
分支则都不就绪时立即返回，常用于非阻塞收发与超时控制：

    select {
    case v := <-ch1:
    case <-time.After(time.Second):
    }

## 常见坑

1. 主 goroutine 退出会带走所有子 goroutine——需要 `sync.WaitGroup` 等待
2. 向已关闭的 channel 发送会 panic——关闭职责只属于唯一的发送方
3. 在循环里启动 goroutine 捕获循环变量：Go 1.22 起循环变量每轮独立，
   老版本需要 `v := v` 快照
4. goroutine 泄漏：阻塞在无人接收的 channel 上——用 context 取消或超时兜底

## 经验法则

能用品级并发达到的就别上协程级复杂度；并发原语首选 channel，
共享状态保护次选 `sync.Mutex`。
```

`docs/notes/mcp-protocol.md`：

```markdown
# MCP（Model Context Protocol）协议笔记

## 是什么

MCP 是 2024 年 11 月由 Anthropic 发布的开放协议，为 LLM 应用与外部
工具/数据源之间定义标准连接方式：服务器暴露能力，客户端（通常是
Agent/IDE）发现并调用。常被类比成"AI 应用的 USB-C 接口"。

## 传输层

- stdio：客户端把服务器作为子进程拉起，stdin/stdout 交换消息，本地最快
- Streamable HTTP：远程部署，HTTP POST + 可选 SSE 流

## 三类原语

1. tools：可被模型调用的函数（function calling 的服务端）
2. resources：只读数据源（文件、数据库行等）
3. prompts：可复用的提示词模板

## 协议细节

- 消息格式：JSON-RPC 2.0，请求/响应/通知三种
- 握手：客户端先发 initialize（带 protocolVersion 与 clientInfo），
  服务器回应能力声明，客户端再发 notifications/initialized 完成握手
- 工具发现：tools/list；工具调用：tools/call（name + arguments）

## 与 function calling 的关系

MCP 客户端把 tools/list 拿到的 JSON Schema 转换成 OpenAI function
calling 格式喂给模型；模型的 tool_calls 再经 tools/call 送回服务器。
MCP 负责"工具在哪里、怎么调"，function calling 负责"模型怎么决定调"。
```

`docs/notes/rag-basics.md`：

```markdown
# RAG（检索增强生成）基础

## 为什么需要 RAG

大模型的参数化知识有截止日期且不可私有。RAG 的思路：先把相关资料
检索出来，拼进上下文，再让模型"看着资料答题"，从而把答案锚定在
事实之上、降低幻觉。

## 标准流程

加载文档 → 切分（chunking）→ 嵌入（embedding）→ 存入向量库 →
查询时：问题向量化 → 相似度检索 top-k → 拼上下文 → 生成引用式回答。

## 切分策略

固定字符窗口 + 重叠（overlap）是最简单的教学策略：每块 500 字符、
相邻块重叠 80 字符。重叠防止关键句被切断在块边界；生产中更常用
按标题/段落的语义切分。块太大稀释相关性，太小丢上下文。

## 嵌入与相似度

embedding 把文本映射为高维向量，语义相近则向量夹角小。检索用
余弦相似度排序。中文场景可选 Qwen text-embedding-v4、GLM embedding-3、
开源 bge-m3（硅基流动提供 API）。

## 约束回答质量

system prompt 明确要求"仅基于给定资料回答，标注引用编号，资料不足
就说不知道"——这是抑制幻觉的最后一道闸。
```

`docs/notes/agent-patterns.md`：

```markdown
# Agent 常见模式笔记

## Agent 循环

Agent = LLM + 工具 + 循环：用户输入 → 模型思考 → 返回 tool_calls →
执行工具 → 结果以 tool 角色回填 → 模型继续思考 → …… 直到模型给出
自然语言最终回答。每一轮循环叫一步（step）。

## function calling 协议要点

- 请求：tools 数组声明 name/description/parameters（JSON Schema）
- 响应：finish_reason=tool_calls，携带 tool_calls[].id/function.name/arguments
- 回填：assistant 消息带 tool_calls + 每个调用对应一条 tool 消息
  （tool_call_id 必须配对），模型据此生成下一条消息

## 工具错误是数据不是异常

工具执行失败时，把错误文本（如 "ERROR: 除数为零"）作为工具结果
回填给模型，让模型自己决定重试、换参数还是向用户解释——这比直接
崩溃更符合 Agent 的韧性要求。

## 记忆管理

多轮对话历史无限增长会撑爆上下文：常用策略是截断到最近 N 轮。
注意截断后开头不能是 tool 消息（它引用的 assistant 已被截掉，
协议上非法），需要继续丢弃直到开头不是 tool 消息。

## 步数上限

循环必须有 MaxSteps 保护（如 8 步），防止模型陷入无限调工具。
```

- [ ] **Step 6: 实现 demo01 服务器**

`cmd/01-mcp-server/main.go`：

```go
// demo 01：MCP 服务器（stdio 传输）。
// 提供 get_time / calculate / read_note 三个教学工具。
// 验证方式：printf 管道喂 JSON-RPC（见 README），或用 MCP Inspector 连接。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // Windows 没有系统时区库，内嵌 tzdata 保证 LoadLocation 可用

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"example-go-agent-lab/internal/expr"
)

func main() {
	s := server.NewMCPServer(
		"notes-tools",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	registerGetTime(s)
	registerCalculate(s)
	registerReadNote(s)
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, "服务器退出:", err)
		os.Exit(1)
	}
}

// get_time：获取当前时间，可选 IANA 时区。
func registerGetTime(s *server.MCPServer) {
	tool := mcp.NewTool("get_time",
		mcp.WithDescription("获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。"),
		mcp.WithString("timezone",
			mcp.Description("IANA 时区名"),
			mcp.Enum("Asia/Shanghai", "UTC", "America/New_York", "Europe/London", "Asia/Tokyo"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tz := req.GetString("timezone", "")
		loc := time.Local
		if tz != "" {
			var err error
			loc, err = time.LoadLocation(tz)
			if err != nil {
				return mcp.NewToolResultErrorf("无法识别时区 %q: %v", tz, err), nil
			}
		}
		now := time.Now().In(loc)
		text := fmt.Sprintf("%s（%s，星期%s）",
			now.Format("2006-01-02 15:04:05"), now.Format("MST"),
			string([]rune("日一二三四五六")[int(now.Weekday())]))
		return mcp.NewToolResultText(text), nil
	})
}

// calculate：四则运算表达式求值（安全的递归下降解析，非 eval）。
func registerCalculate(s *server.MCPServer) {
	tool := mcp.NewTool("calculate",
		mcp.WithDescription("计算四则运算表达式的值，支持 + - * /、括号与一元负号，例如 (1+2)*3。"),
		mcp.WithString("expression",
			mcp.Required(),
			mcp.Description("算术表达式，如 1+2*3"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := req.RequireString("expression")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("参数错误", err), nil
		}
		val, err := expr.Evaluate(raw)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("表达式无法计算", err), nil
		}
		// 去掉多余的尾零：9.000000 → 9，2.500000 → 2.5
		text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
		return mcp.NewToolResultText(text), nil
	})
}

// read_note：读取 docs/notes/ 下的笔记全文。
func registerReadNote(s *server.MCPServer) {
	tool := mcp.NewTool("read_note",
		mcp.WithDescription("读取项目知识库笔记的完整内容。name 是 docs/notes/ 下的文件名，如 mcp-protocol.md。"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("笔记文件名（.md 或 .txt）"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("参数错误", err), nil
		}
		// 防路径穿越：只取文件名部分，丢弃任何目录成分
		safe := filepath.Base(filepath.Clean(name))
		switch strings.ToLower(filepath.Ext(safe)) {
		case ".md", ".txt":
		default:
			return mcp.NewToolResultErrorf("仅支持 .md/.txt 文件，got %q", safe), nil
		}
		data, err := os.ReadFile(filepath.Join("docs", "notes", safe))
		if err != nil {
			return mcp.NewToolResultErrorf("读取失败: %v", err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}
```

- [ ] **Step 7: 管道验收（不需要 API key）**

```bash
go build ./... && go vet ./...
printf '%s\n%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp-server
```

Expected: stdout 四行 JSON-RPC 响应——initialize result、（initialized 无响应）、tools/list 含 `"get_time"` `"calculate"` `"read_note"`、tools/call 的 content 里含 `"9"`

- [ ] **Step 8: 提交**

```bash
git add internal/expr cmd/01-mcp-server docs/notes
git commit -m "feat: demo01 MCP 服务器——get_time/calculate/read_note + 安全表达式求值 + 笔记语料"
```

---

### Task 4: internal/mcp2llm — MCP 工具 → OpenAI function calling 转换

**Files:**
- Create: `internal/mcp2llm/convert.go`
- Create: `internal/mcp2llm/convert_test.go`

**Interfaces:**
- Consumes: `mcp.Tool`（v1.2.1）；Task 2 的 `llm.ToolDef` 别名
- Produces: `mcp2llm.ConvertTools(tools []mcp.Tool) []llm.ToolDef`（纯函数，demo 02/04 使用）

- [ ] **Step 1: 写失败测试**

`internal/mcp2llm/convert_test.go`：

```go
package mcp2llm

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"example-go-agent-lab/internal/llm"
)

func TestConvertTools(t *testing.T) {
	tools := []mcp.Tool{
		{
			Name:        "get_time",
			Description: "获取当前时间",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"timezone": map[string]any{"type": "string"},
				},
				Required: []string{"timezone"},
			},
		},
	}
	got := ConvertTools(tools)
	if len(got) != 1 {
		t.Fatalf("got %d 个工具, want 1", len(got))
	}

	// 直接用 union 的 GetFunction 访问器断言
	f := got[0].GetFunction()
	if f == nil {
		t.Fatal("应为 function 类型工具")
	}
	if f.Name != "get_time" {
		t.Errorf("name = %q", f.Name)
	}
	if !f.Description.Valid() || f.Description.Value != "获取当前时间" {
		t.Errorf("description = %+v", f.Description)
	}

	// JSON Schema 字段经序列化断言（这才是发给 API 的最终形状）
	b, err := json.Marshal(f.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(b, &params); err != nil {
		t.Fatal(err)
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v", params["type"])
	}
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["timezone"]; !ok {
		t.Errorf("properties 缺 timezone: %v", params["properties"])
	}
	req, _ := params["required"].([]any)
	if len(req) != 1 || req[0] != "timezone" {
		t.Errorf("required = %v", params["required"])
	}
}

func TestConvertToolsEmptyRequired(t *testing.T) {
	tools := []mcp.Tool{{
		Name:        "ping",
		Description: "",
		InputSchema: mcp.ToolInputSchema{Type: "object", Properties: map[string]any{}},
	}}
	got := ConvertTools(tools)
	b, _ := json.Marshal(got[0].GetFunction().Parameters)
	var params map[string]any
	_ = json.Unmarshal(b, &params)
	if _, exists := params["required"]; exists {
		t.Errorf("无必填参数时不应输出 required 字段: %s", b)
	}
}

var _ llm.ToolDef // 保证包引用（ConvertTools 返回类型即 llm.ToolDef）
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/mcp2llm/`
Expected: FAIL（`undefined: ConvertTools`）

- [ ] **Step 3: 实现转换**

`internal/mcp2llm/convert.go`：

```go
// Package mcp2llm 把 MCP tools/list 返回的工具定义
// 转换成 OpenAI function calling 的 tools 格式（纯函数，可单测）。
package mcp2llm

import (
	openai "github.com/openai/openai-go/v3"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3/shared"

	"example-go-agent-lab/internal/llm"
)

// ConvertTools 批量转换。MCP 的 InputSchema（JSON Schema 对象）
// 直接平移为 function.parameters 的 type/properties/required 字段。
func ConvertTools(tools []mcp.Tool) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		params := shared.FunctionParameters{
			"type":       t.InputSchema.Type,
			"properties": t.InputSchema.Properties,
		}
		if t.InputSchema.Properties == nil {
			params["properties"] = map[string]any{}
		}
		if len(t.InputSchema.Required) > 0 {
			params["required"] = t.InputSchema.Required
		}
		fd := shared.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: params,
		}
		if t.Description != "" {
			fd.Description = openai.String(t.Description)
		}
		out = append(out, openai.ChatCompletionFunctionTool(fd))
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过 + 提交**

```bash
go mod tidy && go build ./... && go vet ./... && go test ./...
git add internal/mcp2llm go.mod go.sum
git commit -m "feat: mcp2llm——MCP 工具 schema 到 function calling 的纯函数转换"
```

Expected: 全绿（mcp-go 进入 go.mod require）

---

### Task 5: internal/agent — 可注入的工具循环

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: Task 2 的 `llm.Message`、`llm.ToolDef`、`llm.ToolCall`、`llm.AssistantWithToolCalls`、`llm.ToolResult`
- Produces:
  - `type ToolHandler func(ctx context.Context, args map[string]any) (string, error)`
  - `type CompleteFn func(ctx context.Context, msgs []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error)`
  - `type Agent struct{SystemPrompt string; ToolDefs []llm.ToolDef; Handlers map[string]ToolHandler; Complete CompleteFn; MaxSteps int; OnToolCall func(name, arguments, result string)}`
  - `const DefaultMaxSteps = 8`
  - `func (a *Agent) Run(ctx context.Context, history []llm.Message, user string) (string, []llm.Message, error)` — 返回最终回答与更新后的历史（不含 system，system 由 Agent 每轮注入）
  - `func TruncateHistory(msgs []llm.Message, keepLast int) []llm.Message`

- [ ] **Step 1: 写失败测试（fake Complete + fake Handler，无网络）**

`internal/agent/agent_test.go`：

```go
package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/llm"
)

// scriptedComplete 按脚本依次返回模型响应，并捕获每次收到的消息列表。
type scriptedComplete struct {
	responses []struct {
		content string
		calls   []llm.ToolCall
	}
	captured [][]llm.Message
}

func (s *scriptedComplete) fn(ctx context.Context, msgs []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error) {
	cp := append([]llm.Message(nil), msgs...)
	s.captured = append(s.captured, cp)
	r := s.responses[len(s.captured)-1]
	return r.content, r.calls, nil
}

func TestRunDirectAnswer(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{{content: "你好！", calls: nil}}}
	ag := &Agent{Complete: sc.fn}
	answer, updated, err := ag.Run(context.Background(), nil, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "你好！" {
		t.Errorf("answer = %q", answer)
	}
	// 更新后的历史：user + assistant，各 1 条；system 不进历史
	if len(updated) != 2 {
		t.Fatalf("len(updated) = %d, want 2", len(updated))
	}
	// 无 SystemPrompt 时，发给模型的首条消息不应是 system
	first := sc.captured[0][0]
	if first.OfSystem != nil {
		t.Errorf("无 SystemPrompt 时首条不应是 system")
	}
}

func TestRunToolRoundTrip(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "call_1", Name: "calculate", Arguments: `{"expression":"1+2"}`}}},
		{content: "1+2=3"},
	}}
	var gotArgs map[string]any
	ag := &Agent{
		SystemPrompt: "测试",
		Handlers: map[string]ToolHandler{
			"calculate": func(ctx context.Context, args map[string]any) (string, error) {
				gotArgs = args
				return "3", nil
			},
		},
		Complete: sc.fn,
	}
	answer, updated, err := ag.Run(context.Background(), nil, "1+2等于几")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "1+2=3" {
		t.Fatalf("answer = %q", answer)
	}
	if gotArgs["expression"] != "1+2" {
		t.Errorf("handler 收到 args = %v", gotArgs)
	}
	// 历史：user、assistant(tool_calls)、tool、assistant(final)
	if len(updated) != 4 {
		t.Fatalf("len(updated) = %d, want 4", len(updated))
	}
	// 第二次调模型的入参里含 tool 结果消息（经 JSON 断言 union 内容）
	second := sc.captured[1]
	b, _ := json.Marshal(second)
	s := string(b)
	if !strings.Contains(s, `"role":"tool"`) || !strings.Contains(s, `"3"`) {
		t.Errorf("第二轮消息缺少 tool 结果: %s", s)
	}
	if !strings.Contains(s, `"tool_call_id":"call_1"`) {
		t.Errorf("tool 消息未带 tool_call_id: %s", s)
	}
}

func TestRunToolErrorIsData(t *testing.T) {
	// 工具执行报错不应中断循环，而是以 ERROR: 文本回填给模型
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "c1", Name: "bad", Arguments: `{}`}}},
		{content: "工具坏了"},
	}}
	ag := &Agent{
		Handlers: map[string]ToolHandler{
			"bad": func(ctx context.Context, args map[string]any) (string, error) {
				return "", context.DeadlineExceeded
			},
		},
		Complete: sc.fn,
	}
	answer, _, err := ag.Run(context.Background(), nil, "q")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "工具坏了" {
		t.Fatalf("answer = %q", answer)
	}
	b, _ := json.Marshal(sc.captured[1])
	if !strings.Contains(string(b), "ERROR:") {
		t.Errorf("工具错误未以 ERROR: 回填: %s", b)
	}
}

func TestRunUnknownTool(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "c1", Name: "nope", Arguments: `{}`}}},
		{content: "没这个工具"},
	}}
	ag := &Agent{Complete: sc.fn}
	if _, _, err := ag.Run(context.Background(), nil, "q"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(sc.captured[1])
	if !strings.Contains(string(b), "未知工具") {
		t.Errorf("未知工具未正确回填: %s", b)
	}
}

func TestRunMaxStepsExceeded(t *testing.T) {
	calls := []llm.ToolCall{{ID: "c1", Name: "loop", Arguments: `{}`}}
	sc := &scriptedComplete{}
	for i := 0; i < 20; i++ {
		sc.responses = append(sc.responses, struct {
			content string
			calls   []llm.ToolCall
		}{calls: calls})
	}
	ag := &Agent{
		Handlers: map[string]ToolHandler{
			"loop": func(ctx context.Context, args map[string]any) (string, error) { return "x", nil },
		},
		Complete: sc.fn,
	}
	if _, _, err := ag.Run(context.Background(), nil, "q"); err == nil {
		t.Fatal("超过最大步数应报错")
	} else if !strings.Contains(err.Error(), "最大步数") {
		t.Fatalf("错误应提及最大步数: %v", err)
	}
}

func TestTruncateHistory(t *testing.T) {
	u := openai.UserMessage("u")
	tcMsg := llm.AssistantWithToolCalls("", []llm.ToolCall{{ID: "1", Name: "n", Arguments: `{}`}})
	toolMsg := llm.ToolResult("r", "1")
	aMsg := openai.AssistantMessage("a")
	msgs := []llm.Message{u, tcMsg, toolMsg, aMsg, toolMsg, u}
	// keepLast=4 截到 [toolMsg, aMsg, toolMsg, u]，开头是 tool 消息被继续丢弃 → [aMsg, toolMsg, u]
	got := TruncateHistory(msgs, 4)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3（开头 tool 消息被继续丢弃）", len(got))
	}
	if got[0].OfAssistant == nil {
		t.Errorf("截断后首条应为 assistant 消息")
	}
	// 不超长时原样返回
	if got2 := TruncateHistory(msgs, 100); len(got2) != len(msgs) {
		t.Errorf("不超长不应截断")
	}
}
```

（注：`openai.AssistantMessage("a")` 签名是泛型 `AssistantMessage[T string|...]`，传字符串即可。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/agent/`
Expected: FAIL（`undefined: Agent` 等）

- [ ] **Step 3: 实现 agent**

`internal/agent/agent.go`：

```go
// Package agent 实现与传输无关的 LLM 工具循环：
// 模型返回 tool_calls → 执行工具 → 结果回填 → 再问模型，直到最终回答。
// Complete 与 Handler 均为可注入接口，单元测试用假实现，不碰真实 API。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/llm"
)

// ToolHandler 是一个工具的执行函数；错误会被当作数据回填给模型。
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// CompleteFn 抽象"把对话交给大模型"这一步（llm.Client.Complete 满足此签名）。
type CompleteFn func(ctx context.Context, msgs []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error)

// DefaultMaxSteps 防止模型无限调工具。
const DefaultMaxSteps = 8

// Agent 一次 Run 处理一轮用户输入。
type Agent struct {
	SystemPrompt string
	ToolDefs     []llm.ToolDef            // 给模型看的工具定义（name/schema）
	Handlers     map[string]ToolHandler   // 工具名 → 执行函数
	Complete     CompleteFn
	MaxSteps     int
	OnToolCall   func(name, arguments, result string) // 工具调用轨迹回调（打印日志用）
}

// Run 处理一轮输入并返回（最终回答, 更新后的历史, 错误）。
// history 不含 system——SystemPrompt 每轮由 Agent 注入头部；
// 更新后的历史包含本轮 user、中间的 assistant/tool 消息与最终 assistant 回答。
func (a *Agent) Run(ctx context.Context, history []llm.Message, user string) (string, []llm.Message, error) {
	maxSteps := a.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	updated := make([]llm.Message, 0, len(history)+4)
	updated = append(updated, history...)
	updated = append(updated, openai.UserMessage(user))

	msgs := make([]llm.Message, 0, len(updated)+1)
	if a.SystemPrompt != "" {
		msgs = append(msgs, openai.SystemMessage(a.SystemPrompt))
	}
	msgs = append(msgs, updated...)

	for step := 0; step < maxSteps; step++ {
		content, calls, err := a.Complete(ctx, msgs, a.ToolDefs)
		if err != nil {
			return "", updated, err
		}
		if len(calls) == 0 {
			updated = append(updated, openai.AssistantMessage(content))
			return content, updated, nil
		}
		// 模型决定调用工具：assistant(tool_calls) 写入两侧消息流
		assistantMsg := llm.AssistantWithToolCalls(content, calls)
		msgs = append(msgs, assistantMsg)
		updated = append(updated, assistantMsg)
		for _, call := range calls {
			result := a.callTool(ctx, call)
			if a.OnToolCall != nil {
				a.OnToolCall(call.Name, call.Arguments, result)
			}
			toolMsg := llm.ToolResult(result, call.ID)
			msgs = append(msgs, toolMsg)
			updated = append(updated, toolMsg)
		}
	}
	return "", updated, fmt.Errorf("已达最大步数 %d 仍未得到最终回答", maxSteps)
}

// callTool 执行单个工具调用。
// 工具层错误（未知工具/参数非法/执行失败）不中断循环，统一以 "ERROR: ..." 回填。
func (a *Agent) callTool(ctx context.Context, call llm.ToolCall) string {
	handler, ok := a.Handlers[call.Name]
	if !ok {
		return fmt.Sprintf("ERROR: 未知工具 %q", call.Name)
	}
	var args map[string]any
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return fmt.Sprintf("ERROR: 工具参数不是合法 JSON: %v", err)
		}
	}
	result, err := handler(ctx, args)
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	return result
}

// TruncateHistory 保留最近 keepLast 条消息，用于控制多轮对话上下文长度。
// 若截断后开头是 tool 消息（其配对的 assistant 已被截掉，协议上非法），
// 会继续丢弃，直到开头不是 tool 消息。
func TruncateHistory(msgs []llm.Message, keepLast int) []llm.Message {
	if len(msgs) <= keepLast {
		return msgs
	}
	out := append([]llm.Message(nil), msgs[len(msgs)-keepLast:]...)
	for len(out) > 0 && out[0].OfTool != nil {
		out = out[1:]
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过 + 提交**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/agent
git commit -m "feat: agent 工具循环——可注入 Complete/Handler、错误即数据、MaxSteps 保护、历史截断"
```

Expected: `ok example-go-agent-lab/internal/agent`

---

### Task 6: demo 02 — MCP Client + LLM 终端 REPL

**Files:**
- Create: `cmd/02-mcp-agent/main.go`

**Interfaces:**
- Consumes: Task 1 全部（config/provider/appenv）、Task 2（llm.New/Complete）、Task 4（mcp2llm.ConvertTools）、Task 5（agent.Agent 等）、demo01 二进制（经 `go run` 子进程）
- Produces: demo02 可执行（验收即产出）

- [ ] **Step 1: 实现 main**

`cmd/02-mcp-agent/main.go`：

```go
// demo 02：MCP 客户端 + LLM 工具循环。
// 拉起 demo01 作为 stdio MCP 子进程，把它的工具转成 function calling
// 交给模型，模型返回 tool_calls 时经 MCP CallTool 执行并回填。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"example-go-agent-lab/internal/agent"
	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/mcp2llm"
	"example-go-agent-lab/internal/provider"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ep, err := provider.Chat(cfg.Provider)
	if err != nil {
		log.Fatalf("聊天 provider 无效: %v", err)
	}
	apiKey, err := appenv.Lookup(ep.APIKeyEnv)
	if err != nil {
		log.Fatal(err)
	}
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = ep.DefaultChatModel
	}
	chat := llm.New(ep.BaseURL, apiKey, chatModel)
	log.Printf("聊天模型: %s（%s）", chatModel, ep.Name)

	ctx := context.Background()

	// 1. 拉起 demo01 MCP 服务器子进程（stdio 传输）
	mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp-server")
	if err != nil {
		log.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpc.Close()
	if err := mcpc.Start(ctx); err != nil {
		fatalWithStderr(mcpc, "启动 MCP 子进程失败: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "02-mcp-agent", Version: "1.0.0"}
	if _, err := mcpc.Initialize(ctx, initReq); err != nil {
		fatalWithStderr(mcpc, "MCP initialize 握手失败: %v", err)
	}

	// 2. 发现工具并转换成 function calling 格式
	toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fatalWithStderr(mcpc, "tools/list 失败: %v", err)
	}
	mcpTools := toolsRes.Tools
	names := make([]string, len(mcpTools))
	for i, t := range mcpTools {
		names[i] = t.Name
	}
	log.Printf("发现 %d 个 MCP 工具: %s", len(mcpTools), strings.Join(names, ", "))

	// 3. 每个工具的执行包装：转发到 MCP CallTool
	handlers := make(map[string]agent.ToolHandler)
	for _, t := range mcpTools {
		handlers[t.Name] = func(ctx context.Context, args map[string]any) (string, error) {
			res, err := mcpc.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: t.Name, Arguments: args},
			})
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					sb.WriteString(tc.Text)
				}
			}
			return sb.String(), nil
		}
	}

	// 4. Agent + REPL
	ag := &agent.Agent{
		SystemPrompt: "你是一个能调用 MCP 工具的中文助手。涉及当前时间、算术计算、项目笔记查询的问题优先使用工具；工具结果出错时向用户如实说明。回答保持简洁。",
		ToolDefs:     mcp2llm.ConvertTools(mcpTools),
		Handlers:     handlers,
		Complete:     chat.Complete,
		MaxSteps:     8,
		OnToolCall: func(name, args, result string) {
			fmt.Printf("  ⚙ %s(%s)\n", name, args)
			fmt.Printf("  ← %s\n", truncateLines(result, 3))
		},
	}

	fmt.Println("MCP Agent 已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []llm.Message
	for {
		fmt.Print("你> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		answer, updated, err := ag.Run(ctx, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

// fatalWithStderr 报错时附带 MCP 子进程的 stderr 摘要，便于定位崩溃原因。
func fatalWithStderr(c *client.Client, format string, args ...any) {
	if r, ok := client.GetStderr(c); ok {
		if tail, err := io.ReadAll(r); err == nil && len(tail) > 0 {
			log.Fatalf(format+"；子进程 stderr:\n%s", append(args, string(tail))...)
		}
	}
	log.Fatalf(format, args...)
}

// truncateLines 把多行文本压成最多 maxLines 行（每行再截 120 字符）。
func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	kept := lines[:maxLines]
	for i, l := range kept {
		r := []rune(l)
		if len(r) > 120 {
			kept[i] = string(r[:120]) + "…"
		}
	}
	return strings.Join(kept, "\n") + fmt.Sprintf("\n  …（共 %d 行，已截断）", len(lines))
}
```

注意：`for _, t := range mcpTools` 中闭包捕获 `t`——Go 1.22 起循环变量按轮次独立，捕获安全。

- [ ] **Step 2: 构建与无 key 验收**

```bash
go build ./... && go vet ./...
# 未设置 key 时应给出指明环境变量的错误：
DEEPSEEK_API_KEY= go run ./cmd/02-mcp-agent < /dev/null
```

Expected: 输出 `缺少环境变量 DEEPSEEK_API_KEY（请先设置，例如: export DEEPSEEK_API_KEY=sk-xxx）` 后退出

- [ ] **Step 3: 真 API 验收（需 DEEPSEEK_API_KEY）**

```bash
export DEEPSEEK_API_KEY=<你的key>
printf '现在几点了？北京时间\n(1+2)*3等于多少\ncalculate 表达式 1/0 会怎样\nexit\n' | go run ./cmd/02-mcp-agent
```

Expected: 前两问出现 `⚙ get_time(...)` / `⚙ calculate(...)` 轨迹并给出正确答案；第三问模型收到 `ERROR: 除数为零` 的工具结果后向用户解释（而非崩溃）——这正是"工具错误是数据"的教学点

- [ ] **Step 4: 提交**

```bash
git add cmd/02-mcp-agent
git commit -m "feat: demo02 MCP 客户端 + LLM 工具循环 REPL"
```

---

### Task 7: internal/splitter — 固定窗口 + 重叠切分

**Files:**
- Create: `internal/splitter/splitter.go`
- Create: `internal/splitter/splitter_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `splitter.Split(text string, chunkSize, overlap int) ([]string, error)` — 按 rune 计数（对中文友好）；空文本返回 nil

- [ ] **Step 1: 写失败测试**

`internal/splitter/splitter_test.go`：

```go
package splitter

import (
	"reflect"
	"testing"
)

func TestSplitExactChunks(t *testing.T) {
	// 10 个字符，窗口 5，重叠 1，步长 4：[0:5] [4:9] [8:10]
	got, err := Split("0123456789", 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"01234", "45678", "89"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitSingleChunkWhenShort(t *testing.T) {
	got, err := Split("短文本", 500, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"短文本"}) {
		t.Errorf("got %v", got)
	}
}

func TestSplitChineseRunes(t *testing.T) {
	// 按 rune 而非字节切：8 个汉字，窗口 4，重叠 1
	got, err := Split("一二三四五六七八", 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"一二三四", "四五六七", "七八"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitEmpty(t *testing.T) {
	got, err := Split("", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("空文本应返回 nil, got %v", got)
	}
}

func TestSplitInvalidArgs(t *testing.T) {
	for _, c := range [][2]int{{0, 0}, {-1, 0}, {5, -1}, {5, 5}, {3, 10}} {
		if _, err := Split("abc", c[0], c[1]); err == nil {
			t.Errorf("Split(size=%d, overlap=%d) 应报错", c[0], c[1])
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/splitter/`
Expected: FAIL（`undefined: Split`）

- [ ] **Step 3: 实现**

`internal/splitter/splitter.go`：

```go
// Package splitter 实现最简单的教学切分策略：固定 rune 窗口 + 重叠。
// 按 rune 计数保证中文不被从字节中间切断；
// overlap 让相邻块共享一段文本，防止关键句正好落在块边界被切断。
package splitter

import "fmt"

// Split 把 text 切成若干块：每块至多 chunkSize 个 rune，
// 相邻块重叠 overlap 个 rune。空文本返回 nil。
func Split(text string, chunkSize, overlap int) ([]string, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize 必须为正数, got %d", chunkSize)
	}
	if overlap < 0 || overlap >= chunkSize {
		return nil, fmt.Errorf("overlap 必须满足 0 <= overlap < chunkSize, got overlap=%d chunkSize=%d", overlap, chunkSize)
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, nil
	}
	step := chunkSize - overlap
	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks, nil
}
```

- [ ] **Step 4: 运行确认通过 + 提交**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/splitter
git commit -m "feat: splitter——固定 rune 窗口 + 重叠的文本切分"
```

---

### Task 8: demo 03 — RAG（ingest / ask）

**Files:**
- Create: `cmd/03-rag/main.go`

**Interfaces:**
- Consumes: Task 1（config/provider/appenv）、Task 2（llm.New/NewEmbedder/Complete）、Task 7（splitter.Split）、chromem-go、docs/notes 语料（Task 3）
- Produces: demo03 可执行；`.chromem/` 持久化向量库（已 gitignore）

- [ ] **Step 1: 实现 main（子命令 ingest / ask）**

`cmd/03-rag/main.go`：

```go
// demo 03：检索增强问答（RAG）。
// ingest：加载 docs/notes 语料 → 切分 → embedding → chromem-go 持久化
// ask：问题向量化 → top-k 检索 → 拼上下文 → LLM 生成带引用的回答
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/philippgille/chromem-go"
	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/provider"
	"example-go-agent-lab/internal/splitter"
)

const (
	chunkSize = 500 // 每块最大 rune 数
	overlap   = 80  // 相邻块重叠 rune 数
	topK      = 3   // 检索返回块数
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || (args[0] != "ingest" && args[0] != "ask") {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/03-rag <ingest|ask>")
		fmt.Fprintln(os.Stderr, "  ingest          建库（docs/notes → .chromem/）")
		fmt.Fprintln(os.Stderr, "  ask <问题...>   基于知识库问答")
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}

	// embedding 端点独立于聊天端点（deepseek 无 embedding 接口）
	eep, err := provider.Embedding(cfg.EmbeddingProvider)
	if err != nil {
		log.Fatalf("embedding provider 无效: %v", err)
	}
	ekey, err := appenv.Lookup(eep.APIKeyEnv)
	if err != nil {
		log.Fatal(err)
	}
	emodel := cfg.EmbeddingModel
	if emodel == "" {
		emodel = eep.DefaultEmbeddingModel
	}
	embedder := llm.NewEmbedder(eep.BaseURL, ekey, emodel)
	log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)

	// chromem-go 的 EmbeddingFunc 适配器：内部走我们的 OpenAI 兼容 Embedder
	ef := chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
		vs, err := embedder.Embed(ctx, []string{text})
		if err != nil {
			return nil, err
		}
		return vs[0], nil
	})

	db, err := chromem.NewPersistentDB(".chromem", false)
	if err != nil {
		log.Fatalf("打开向量库失败: %v", err)
	}
	col, err := db.GetOrCreateCollection("notes", nil, ef)
	if err != nil {
		log.Fatalf("创建集合失败: %v", err)
	}

	ctx := context.Background()
	if args[0] == "ingest" {
		if err := ingest(ctx, col); err != nil {
			log.Fatalf("ingest 失败: %v", err)
		}
		return
	}
	question := strings.Join(args[1:], " ")
	if strings.TrimSpace(question) == "" {
		log.Fatal("ask 需要跟一个问题，例如: go run ./cmd/03-rag ask MCP 是什么")
	}
	if err := ask(ctx, col, cfg, question); err != nil {
		log.Fatalf("ask 失败: %v", err)
	}
}

// ingest 建库：ID 取 "文件名#序号"，重复 ingest 按相同 ID 覆盖更新（幂等）。
func ingest(ctx context.Context, col *chromem.Collection) error {
	var docs []chromem.Document
	for _, pat := range []string{"docs/notes/*.md", "docs/notes/*.txt"} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return fmt.Errorf("读取 %s: %w", f, err)
			}
			chunks, err := splitter.Split(string(data), chunkSize, overlap)
			if err != nil {
				return fmt.Errorf("切分 %s: %w", f, err)
			}
			base := filepath.Base(f)
			for i, ch := range chunks {
				docs = append(docs, chromem.Document{
					ID:       fmt.Sprintf("%s#%04d", base, i),
					Content:  ch,
					Metadata: map[string]string{"source": base},
				})
			}
			log.Printf("已切分 %s: %d 块", base, len(chunks))
		}
	}
	if len(docs) == 0 {
		return fmt.Errorf("docs/notes/ 下没有找到 .md/.txt 语料")
	}
	if err := col.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		return fmt.Errorf("写入向量库: %w", err)
	}
	log.Printf("入库完成：共 %d 块（已持久化到 .chromem/）", len(docs))
	return nil
}

// ask 检索 + 生成。
func ask(ctx context.Context, col *chromem.Collection, cfg config.Config, question string) error {
	results, err := col.Query(ctx, question, topK, nil, nil)
	if err != nil {
		return fmt.Errorf("检索失败: %w", err)
	}
	if len(results) == 0 {
		fmt.Println("未命中任何资料。请先运行: go run ./cmd/03-rag ingest")
		return nil
	}

	var ctxSB strings.Builder
	for i, r := range results {
		fmt.Fprintf(&ctxSB, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n",
			i+1, r.Metadata["source"], r.Similarity, r.Content)
	}

	ep, err := provider.Chat(cfg.Provider)
	if err != nil {
		return err
	}
	key, err := appenv.Lookup(ep.APIKeyEnv)
	if err != nil {
		return err
	}
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = ep.DefaultChatModel
	}
	chat := llm.New(ep.BaseURL, key, chatModel)

	system := "你是知识库问答助手。只依据下面提供的资料回答问题；引用资料时标注编号（如 [1]）；" +
		"如果资料不足以回答，明确说明资料未覆盖，不要编造。\n\n资料：\n" + ctxSB.String()
	answer, _, err := chat.Complete(ctx, []llm.Message{
		openai.SystemMessage(system),
		openai.UserMessage(question),
	}, nil)
	if err != nil {
		return err
	}

	fmt.Printf("助手> %s\n\n", answer)
	fmt.Println("命中块：")
	for i, r := range results {
		fmt.Printf("  [%d] %s（相似度 %.3f）\n", i+1, r.Metadata["source"], r.Similarity)
	}
	return nil
}
```

- [ ] **Step 2: 构建 + 用法验收（无需 key）**

```bash
go build ./... && go vet ./...
go run ./cmd/03-rag 2>&1 | head -1
```

Expected: `用法: go run ./cmd/03-rag <ingest|ask>` 且退出码非 0

- [ ] **Step 3: 真 API 验收（需 DASHSCOPE_API_KEY + DEEPSEEK_API_KEY）**

```bash
export DEEPSEEK_API_KEY=<key> DASHSCOPE_API_KEY=<key>
go run ./cmd/03-rag ingest
go run ./cmd/03-rag ask "MCP 和 function calling 是什么关系？"
go run ./cmd/03-rag ask "今天股市怎么样"
```

Expected: ingest 打印每个文件的切块数与总数；第一问答案带 `[1]` 类引用且命中块列表非空；第二问（语料外）模型回答"资料未覆盖"而非编造

- [ ] **Step 4: 提交**

```bash
git add cmd/03-rag
git commit -m "feat: demo03 RAG——chromem-go 向量库 + ingest/ask 子命令 + 引用式回答"
```

---

### Task 9: demo 04 综合智能体 + README + 原理文档

**Files:**
- Create: `cmd/04-combo-agent/main.go`
- Create: `README.md`
- Create: `docs/mcp-server.md`、`docs/mcp-agent.md`、`docs/rag.md`、`docs/combo-agent.md`

**Interfaces:**
- Consumes: 全部前置任务（Task 1/2/4/5/8 的包与 demo01/03 的产物）
- Produces: demo04 可执行；项目 README；四篇原理文档

- [ ] **Step 1: 实现 demo04 main（RAG 工具化 + 工具组开关 + 记忆截断）**

`cmd/04-combo-agent/main.go`：

```go
// demo 04：综合智能体 = MCP 工具循环 + RAG 工具化。
// 关键设计：知识库检索不是固定的前置流程，而是被包装为
// search_knowledge_base 工具，与 MCP 工具统一注册，由模型自主决定何时检索。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3/shared"
	"github.com/philippgille/chromem-go"
	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/agent"
	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/mcp2llm"
	"example-go-agent-lab/internal/provider"
)

const topK = 3

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	useMCP := flag.Bool("mcp", true, "启用 MCP 工具组（demo01 的 get_time/calculate/read_note）")
	useRAG := flag.Bool("rag", true, "启用知识库检索工具 search_knowledge_base")
	keepHistory := flag.Int("history", 12, "多轮记忆保留的最近消息条数")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ctx := context.Background()

	ep, err := provider.Chat(cfg.Provider)
	if err != nil {
		log.Fatalf("聊天 provider 无效: %v", err)
	}
	apiKey, err := appenv.Lookup(ep.APIKeyEnv)
	if err != nil {
		log.Fatal(err)
	}
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = ep.DefaultChatModel
	}
	chat := llm.New(ep.BaseURL, apiKey, chatModel)
	log.Printf("聊天模型: %s（%s）", chatModel, ep.Name)

	toolDefs := []llm.ToolDef{}
	handlers := map[string]agent.ToolHandler{}
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	// ── 工具组一：MCP（demo01 子进程）──────────────────────────
	if *useMCP {
		mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp-server")
		if err != nil {
			log.Fatalf("创建 MCP 客户端失败: %v", err)
		}
		closers = append(closers, mcpc)
		if err := mcpc.Start(ctx); err != nil {
			fatalWithStderr(mcpc, "启动 MCP 子进程失败: %v", err)
		}
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "04-combo-agent", Version: "1.0.0"}
		if _, err := mcpc.Initialize(ctx, initReq); err != nil {
			fatalWithStderr(mcpc, "MCP initialize 握手失败: %v", err)
		}
		toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			fatalWithStderr(mcpc, "tools/list 失败: %v", err)
		}
		for _, t := range toolsRes.Tools {
			log.Printf("注册 MCP 工具: %s", t.Name)
		}
		toolDefs = append(toolDefs, mcp2llm.ConvertTools(toolsRes.Tools)...)
		for _, t := range toolsRes.Tools {
			handlers[t.Name] = func(ctx context.Context, args map[string]any) (string, error) {
				res, err := mcpc.CallTool(ctx, mcp.CallToolRequest{
					Params: mcp.CallToolParams{Name: t.Name, Arguments: args},
				})
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, c := range res.Content {
					if tc, ok := c.(mcp.TextContent); ok {
						sb.WriteString(tc.Text)
					}
				}
				return sb.String(), nil
			}
		}
	}

	// ── 工具组二：RAG 工具化 ────────────────────────────────────
	if *useRAG {
		eep, err := provider.Embedding(cfg.EmbeddingProvider)
		if err != nil {
			log.Fatalf("embedding provider 无效: %v", err)
		}
		ekey, err := appenv.Lookup(eep.APIKeyEnv)
		if err != nil {
			log.Fatal(err)
		}
		emodel := cfg.EmbeddingModel
		if emodel == "" {
			emodel = eep.DefaultEmbeddingModel
		}
		embedder := llm.NewEmbedder(eep.BaseURL, ekey, emodel)
		log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)

		ef := chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
			vs, err := embedder.Embed(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			return vs[0], nil
		})
		db, err := chromem.NewPersistentDB(".chromem", false)
		if err != nil {
			log.Fatalf("打开向量库失败: %v", err)
		}
		col, err := db.GetOrCreateCollection("notes", nil, ef)
		if err != nil {
			log.Fatalf("创建集合失败: %v", err)
		}

		toolDefs = append(toolDefs, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: "search_knowledge_base",
			Description: openai.String(
				"在项目知识库（Go 并发、MCP 协议、RAG 基础、Agent 模式等中文笔记）中检索相关资料。" +
					"当用户的问题可能涉及这些主题时调用；闲聊或常识问题不必调用。"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": shared.FunctionParameters{
					"query": shared.FunctionParameters{
						"type":        "string",
						"description": "检索关键词或问题",
					},
				},
				"required": []string{"query"},
			},
		}))
		handlers["search_knowledge_base"] = func(ctx context.Context, args map[string]any) (string, error) {
			q, _ := args["query"].(string)
			if strings.TrimSpace(q) == "" {
				return "", fmt.Errorf("缺少 query 参数")
			}
			results, err := col.Query(ctx, q, topK, nil, nil)
			if err != nil {
				return "", err
			}
			if len(results) == 0 {
				return "未命中任何资料（知识库可能尚未 ingest，可提示用户先运行 go run ./cmd/03-rag ingest）。", nil
			}
			var sb strings.Builder
			for i, r := range results {
				fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n",
					i+1, r.Metadata["source"], r.Similarity, r.Content)
			}
			return sb.String(), nil
		}
		log.Printf("注册 RAG 工具: search_knowledge_base")
	}

	if len(toolDefs) == 0 {
		log.Fatal("至少启用一个工具组（-mcp 或 -rag）")
	}

	ag := &agent.Agent{
		SystemPrompt: "你是一个综合智能体，可以：查时间、算算术、读笔记（MCP 工具），以及检索项目知识库（search_knowledge_base）。" +
			"自主判断何时调用哪个工具；涉及项目笔记主题（Go 并发/MCP/RAG/Agent 模式）时优先检索知识库；" +
			"工具结果出错时向用户如实说明。用中文简洁回答。",
		ToolDefs: toolDefs,
		Handlers: handlers,
		Complete: chat.Complete,
		MaxSteps: 8,
		OnToolCall: func(name, args, result string) {
			fmt.Printf("  ⚙ %s(%s)\n", name, args)
			fmt.Printf("  ← %s\n", firstLine(result, 120))
		},
	}

	fmt.Println("综合智能体已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []llm.Message
	for {
		fmt.Print("你> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		answer, updated, err := ag.Run(ctx, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		// 多轮记忆：截断到最近 N 条（demo02 不截断，这里是增量教学点）
		history = agent.TruncateHistory(updated, *keepHistory)
		fmt.Printf("助手> %s\n", answer)
	}
}

func fatalWithStderr(c *client.Client, format string, args ...any) {
	if r, ok := client.GetStderr(c); ok {
		if tail, err := io.ReadAll(r); err == nil && len(tail) > 0 {
			log.Fatalf(format+"；子进程 stderr:\n%s", append(args, string(tail))...)
		}
	}
	log.Fatalf(format, args...)
}

func firstLine(s string, maxRunes int) string {
	line := strings.SplitN(s, "\n", 2)[0]
	r := []rune(line)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return line
}
```

import 说明：`openai "github.com/openai/openai-go/v3"` 与 `"github.com/openai/openai-go/v3/shared"` 是两个不同包，各导入一次即可。

- [ ] **Step 2: 构建 + 真 API 验收（需两个 key）**

```bash
go build ./... && go vet ./...
export DEEPSEEK_API_KEY=<key> DASHSCOPE_API_KEY=<key>
go run ./cmd/03-rag ingest   # 先建库（demo04 复用同一 .chromem）
printf 'RAG 的切分策略有哪些讲究？\n帮我算一下 (12+8)*5\nexit\n' | go run ./cmd/04-combo-agent
go run ./cmd/04-combo-agent -rag=false   # 只留 MCP 工具组
```

Expected: 第一问触发 `⚙ search_knowledge_base(...)` 且答案含引用语义；第二问触发 `⚙ calculate(...)` 得 100；`-rag=false` 时启动日志只显示 MCP 工具组、问 RAG 问题不触发检索工具

- [ ] **Step 3: 写 README.md（完整正文如下）**

`README.md`：

```markdown
# example-go-agent-lab

用 Go 实现「Agent｜MCP+LLM+RAG｜智能体」的四步递进 demo。
与手写无框架的姊妹项目不同，本项目**以官方/社区库为主**（mcp-go、openai-go、
chromem-go），贴近生产实践。

## 运行顺序

| # | demo | 命令 | 演示什么 |
|---|------|------|----------|
| 01 | MCP 服务器 | `go run ./cmd/01-mcp-server` | 用 mcp-go 写一个 stdio MCP server，暴露 3 个工具 |
| 02 | MCP Agent | `go run ./cmd/02-mcp-agent` | 拉起 01 为子进程，工具接进 LLM 的 function calling 循环 |
| 03 | RAG | `go run ./cmd/03-rag ingest` 然后 `go run ./cmd/03-rag ask "问题"` | chromem-go 向量库 + 引用式问答 |
| 04 | 综合智能体 | `go run ./cmd/04-combo-agent` | RAG 工具化：检索成为模型可自主调用的工具之一 |

每个 demo 的原理说明见 `docs/` 下对应文档。

## 环境变量（key 不落盘）

按所用 provider 设置（见下表），不需要的可以不设：

| 变量 | 用途 |
|------|------|
| `DEEPSEEK_API_KEY` | 聊天（deepseek） |
| `ZHIPU_API_KEY` | 聊天/Embedding（glm） |
| `DASHSCOPE_API_KEY` | 聊天/Embedding（qwen） |
| `SILICONFLOW_API_KEY` | Embedding（siliconflow） |

Windows PowerShell：`$env:DEEPSEEK_API_KEY="sk-..."`；Git Bash：`export DEEPSEEK_API_KEY=sk-...`

## Provider 切换

`configs/config.yaml`：

​```yaml
provider: deepseek        # 聊天：deepseek | glm | qwen
embedding_provider: qwen  # 嵌入：qwen | glm | siliconflow（deepseek 无 embedding 接口）
# chat_model: deepseek-chat        # 可选：覆盖默认模型名
# embedding_model: text-embedding-v4
​```

默认端点与模型见 `internal/provider/provider.go`。

## 快速验证（不需要 API key）

demo01 是纯本地 stdio 服务器，可以手工喂 JSON-RPC 验证：

​```bash
printf '%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp-server
​```

也可以用官方调试器：`npx @modelcontextprotocol/inspector go run ./cmd/01-mcp-server`

## 预期输出示例（demo 02）

​```text
你> 现在几点了？北京时间
  ⚙ get_time({"timezone":"Asia/Shanghai"})
  ← 2026-10-11 15:04:05（CST，星期日）
助手> 现在是北京时间 2026-10-11 15:04:05，星期日。
你> calculate 1/0 会怎样
  ⚙ calculate({"expression":"1/0"})
  ← ERROR: 表达式无法计算: 除数为零
助手: 计算器报告了除数为零……
​```

（时间与模型措辞以实际运行为准；重点看：工具调用轨迹、错误作为工具结果回填而非崩溃。）

## 测试

​```bash
go test ./...
​```

纯逻辑（表达式求值、schema 转换、agent 循环、文本切分、配置加载）全部有单测；
不对真实 API 做单测。

## 目录结构

​```text
cmd/          四个递进 demo
internal/     可复用组件（provider/llm/expr/mcp2llm/agent/splitter/config/appenv）
docs/         原理说明 + notes 知识库语料
configs/      config.yaml
.chromem/     向量库持久化（gitignore，ingest 后生成）
​```

## 非目标

Web UI、流式输出、独立向量数据库、多 Agent 协作——这些主题由姊妹项目（手写实现）覆盖。
```

（注意：README 内嵌代码块用 ` ``` ` 常规写法，上文中的 `​```` 转义仅为在本计划文档里嵌套展示，落盘时写正常围栏。）

- [ ] **Step 4: 写四篇原理文档**

每篇 60-120 行中文，结构统一：**是什么 → 代码怎么走的 → 关键教学点 → 怎么跑/怎么验证**。各篇必须覆盖的具体事实：

`docs/mcp-server.md`：
- mcp-go 的 server 包：NewMCPServer/AddTool/ServeStdio 三个调用即成服务器；stdio 传输的工作方式（stdin 读请求、stdout 写响应）
- 三个工具各自的教学点：get_time 的 `mcp.Enum` 参数约束与 `time/tzdata`（Windows 必需）；calculate 的手写词法+递归下降（为何不用 eval）；read_note 的 `filepath.Base` 防路径穿越
- 工具级错误（NewToolResultError*，置 IsError）与传输级错误（返回 error）的区别
- 验证命令：printf 管道与 MCP Inspector（照抄 README 的命令）

`docs/mcp-agent.md`：
- stdio 子进程模型：client.NewStdioMCPClient 拉起 demo01；Start → Initialize 握手 → tools/list
- mcp2llm.ConvertTools 做了什么（InputSchema → function.parameters 三字段平移）
- agent.Run 循环：user → tool_calls → CallTool → tool 消息回填 → 最终回答；MaxSteps=8
- "工具错误是数据"：demo 里问 1/0 的行为
- 为何 Complete/Handler 做成可注入（单测无网络）

`docs/rag.md`：
- 流程图（文字版）：docs/notes → Split(500,80) → Embedder → chromem 持久化 → Query top-3 → 拼 system → 引用式回答
- chromem-go 的 EmbeddingFunc 适配器（float64→float32）
- ID 幂等设计（文件名#序号，重复 ingest 覆盖）
- 相似度含义（余弦，chromem.Result.Similarity）；空检索的"未命中"语义
- "仅基于资料"system prompt 为何是抑制幻觉的关键

`docs/combo-agent.md`：
- RAG 工具化 vs 固定 RAG 前置流程的对比（模型自主决策检索时机）
- search_knowledge_base 的 schema 与 description 写法对模型决策的影响
- 工具组开关（-mcp/-rag）与记忆截断（-history 12，TruncateHistory 的 tool 消息保护）
- 与 demo02/03 的代码复用关系（同一个 agent.Agent、同一个向量库目录）

- [ ] **Step 5: 收尾检查 + 提交**

```bash
go mod tidy && go build ./... && go vet ./... && go test ./...
git add cmd/04-combo-agent README.md docs/
git commit -m "feat: demo04 综合智能体（RAG 工具化+工具组开关+记忆截断）+ README + 原理文档"
```

Expected: 全部测试绿；`git status` 干净

---

## 最终验收清单（人工过一遍）

1. `go test ./...` 全绿；`go vet ./...` 无告警
2. demo01 管道验收返回 3 个工具与正确计算结果（Task 3 Step 7）
3. 设置 key 后：demo02 两轮工具问答 + 一次工具错误场景；demo03 ingest/ask/语料外问题；demo04 混合提问 + `-rag=false`
4. `grep -r "sk-" --include="*.go" --include="*.yaml" .` 无输出（没有 key 落盘）
5. README 命令全部可复制执行

# 教学版重写 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把项目重写为简写教学版：四步递进 demo（01-MCP / 02-LLM / 03-RAG / 04-Agent 整合），全部用库的最小实现，每个 demo 自包含一个 main 包。

**Architecture:** 每个 demo 是独立的 `package main`，主文件从上往下读完即懂；唯一共享包是 `internal/config`（provider 端点表 + key 解析）。demo 间刻意允许少量复制（工具循环、求值器）——教学项目宁可重复，不引抽象。所有库 API 用法以旧版实现为范例（同版本已验证可用）。

**Tech Stack:** Go 1.26.4；mcp-go v1.2.1、openai-go/v3 v3.76.0、chromem-go v0.7.0、yaml.v3 v3.0.1（版本不动）。

**Spec:** `docs/superpowers/specs/2026-10-14-teaching-rewrite-design.md`

## Global Constraints

- go.mod / go.sum **一个字都不改**，也**不运行 `go mod tidy`**（四个直接依赖最终仍全部在用）。
- 提交前 `gofmt -l .` 必须为空（`*.go` 已被 .gitattributes 锁为 LF）。
- 注释与文档全部中文；教学优先：每个文件顶部注释说清"这一步演示什么"。
- 共享包仅 `internal/config`；demo 间复制代码时**必须**在复制处注释"复制自 demoXX——教学项目宁复制，不抽象"。
- 不对真实 API 做单测；API key 绝不出现在被 git 跟踪的文件里。
- 每任务一次提交，提交信息格式 `type: 中文描述`（type 用 feat/chore/docs/test）。
- 所有可运行命令都在仓库根目录执行。

## Review Focus

下面五类输入/失败模式是 spec 隐含但容易踩的坑，各自的测试钉在对应任务里：

1. **工具错误必须作为数据回填给模型，不能崩溃**——`evaluate("1/0")` 报错（Task 3 `TestEvaluateInvalid`）；dispatch 收到未知工具/坏 JSON（Task 4 `TestDispatch*`）；MCP 工具级错误 `IsError`（Task 3 `call` 的第三个演示调用 + Task 6 handler 转文本）。
2. **聊天循环必须有步数上限**——模型永远返回 tool_calls 时 `runTurn` 报错退出而非死循环（Task 4 `TestRunTurnMaxSteps`）。
3. **模型可能返回空 arguments 字符串**——`dispatch` 对 `""`/空白要当空参数处理而非 JSON 报错（Task 4 `TestDispatchEmptyArguments`）。
4. **RAG 空库**——未 ingest 就 ask/检索时给"未命中"提示，不能触发 chromem 的 `nResults > Count` 报错（Task 5 `TestRetrieveEmptyCollection`；Task 6 的 search_notes 复制同一钳制）。
5. **非法时区字符串**——`get_time("Mars/Phobos")` 返回 ERROR 文本而非 panic（Task 3 `TestTimeIn`）。

---

### Task 1: 清场——移除旧实现

**Files:**
- Delete: `cmd/`（4 个旧 demo）、`internal/`（8 个旧包）、`docs/mcp-server.md`、`docs/mcp-agent.md`、`docs/rag.md`、`docs/combo-agent.md`
- Keep: `configs/`、`docs/notes/`、`docs/superpowers/`、`README.md`（Task 7 重写）、`go.mod`、`go.sum`、`.gitattributes`、`.gitignore`

**Interfaces:**
- Consumes: 无
- Produces: 空仓基线；Task 2 起在空地上重建

- [ ] **Step 1: 删除旧代码与旧文档**

```bash
git rm -r cmd internal
git rm docs/mcp-server.md docs/mcp-agent.md docs/rag.md docs/combo-agent.md
```

- [ ] **Step 2: 验证空仓可构建**

Run: `go build ./... && go vet ./...`
Expected: 无输出，退出码 0（没有任何 Go 包时两者都静默成功）

- [ ] **Step 3: 提交**

```bash
git commit -m "chore: 移除旧版 cmd 与 internal——教学版重写起点（历史见 git log）"
```

---

### Task 2: internal/config——唯一共享包

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `configs/config.yaml`（注释里 provider 表的新位置）、`configs/config.local.yaml.example`（对齐模板）

**Interfaces:**
- Consumes: 无
- Produces（Task 4/5/6 依赖，签名精确到字段）:
  - `config.Load(path string) (Config, error)`——读 yaml；若存在 `<名>.local.yaml` 叠加其 `api_keys`；`Provider` 空默认 `"deepseek"`，`EmbeddingProvider` 空默认 `"qwen"`
  - `type Config struct { Provider, EmbeddingProvider, ChatModel, EmbeddingModel string; APIKeys map[string]string }`
  - `func (c Config) Chat() (ep Endpoint, apiKey, model string, err error)`
  - `func (c Config) Embedding() (ep Endpoint, apiKey, model string, err error)`
  - `type Endpoint struct { Name, BaseURL, APIKeyEnv, DefaultChatModel, DefaultEmbeddingModel string }`
  - key 解析优先级：环境变量 > `c.APIKeys[envName]`；都没有时报错指明两种配置途径

- [ ] **Step 1: 写失败测试**

`internal/config/config_test.go`：

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCfg 在临时目录写 config.yaml 与可选的 config.local.yaml，返回主文件路径。
func writeCfg(t *testing.T, main, local string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if local != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.local.yaml"), []byte(local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "config.yaml")
}

func TestLoadLocalOverride(t *testing.T) {
	path := writeCfg(t,
		"provider: glm\nembedding_provider: qwen\n",
		"api_keys:\n  ZHIPU_API_KEY: sk-local\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "glm" {
		t.Errorf("provider = %q", cfg.Provider)
	}
	if cfg.APIKeys["ZHIPU_API_KEY"] != "sk-local" {
		t.Errorf("本地覆盖未生效: %v", cfg.APIKeys)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeCfg(t, "provider: glm\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingProvider != "qwen" {
		t.Errorf("embedding_provider 默认应为 qwen, got %q", cfg.EmbeddingProvider)
	}
}

func TestChatEnvPriority(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "sk-env")
	cfg := Config{Provider: "glm"}
	ep, key, model, err := cfg.Chat()
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-env" {
		t.Errorf("环境变量应优先于本地文件, key = %q", key)
	}
	if ep.BaseURL != "https://open.bigmodel.cn/api/paas/v4" || model != "glm-4.6" {
		t.Errorf("端点/模型解析错误: %+v %q", ep, model)
	}
}

func TestChatMissingKey(t *testing.T) {
	os.Unsetenv("ZHIPU_API_KEY")
	cfg := Config{Provider: "glm", APIKeys: map[string]string{"DEEPSEEK_API_KEY": "x"}}
	if _, _, _, err := cfg.Chat(); err == nil {
		t.Error("缺少 key 应报错")
	}
}

func TestChatUnknownProvider(t *testing.T) {
	cfg := Config{Provider: "nope"}
	if _, _, _, err := cfg.Chat(); err == nil {
		t.Error("未知 provider 应报错")
	}
}

func TestEmbeddingNoEmbeddingModel(t *testing.T) {
	cfg := Config{EmbeddingProvider: "deepseek"}
	if _, _, _, err := cfg.Embedding(); err == nil {
		t.Error("deepseek 无 embedding 模型应报错")
	}
}

func TestChatModelOverride(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "k")
	cfg := Config{Provider: "glm", ChatModel: "glm-5"}
	_, _, model, err := cfg.Chat()
	if err != nil {
		t.Fatal(err)
	}
	if model != "glm-5" {
		t.Errorf("model = %q, want glm-5", model)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/`
Expected: FAIL（`Load` 等未定义，编译错误）

- [ ] **Step 3: 写实现**

`internal/config/config.go`：

```go
// Package config 是本项目唯一的共享包：
// OpenAI 兼容服务的端点表 + 配置加载（config.yaml + 本地覆盖 config.local.yaml）。
// 四个 demo 里凡是要连 LLM/embedding 的，都从这里拿端点、key 和模型名。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Endpoint 描述一个 OpenAI 兼容服务端点。
// DefaultEmbeddingModel 为空表示该端点不提供 /embeddings（如 deepseek）。
type Endpoint struct {
	Name                  string
	BaseURL               string
	APIKeyEnv             string
	DefaultChatModel      string
	DefaultEmbeddingModel string
}

// endpoints 聊天与 embedding 共用一张表。
var endpoints = map[string]Endpoint{
	"deepseek":    {Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKeyEnv: "DEEPSEEK_API_KEY", DefaultChatModel: "deepseek-chat"},
	"glm":         {Name: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIKeyEnv: "ZHIPU_API_KEY", DefaultChatModel: "glm-4.6", DefaultEmbeddingModel: "embedding-3"},
	"qwen":        {Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKeyEnv: "DASHSCOPE_API_KEY", DefaultChatModel: "qwen-plus", DefaultEmbeddingModel: "text-embedding-v4"},
	"siliconflow": {Name: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1", APIKeyEnv: "SILICONFLOW_API_KEY", DefaultEmbeddingModel: "BAAI/bge-m3"},
}

// Config 是 yaml 配置的形状。ChatModel/EmbeddingModel 空则用端点默认值。
type Config struct {
	Provider          string            `yaml:"provider"`           // 聊天: deepseek | glm | qwen
	EmbeddingProvider string            `yaml:"embedding_provider"` // 嵌入: qwen | glm | siliconflow
	ChatModel         string            `yaml:"chat_model"`         // 可选：覆盖聊天模型名
	EmbeddingModel    string            `yaml:"embedding_model"`    // 可选：覆盖 embedding 模型名
	APIKeys           map[string]string `yaml:"api_keys"`           // 本地覆盖文件专用：环境变量名 → key
}

// Load 读 yaml；若同目录存在 <名>.local.yaml（如 config.yaml → config.local.yaml），
// 再叠加其中的 api_keys。本地文件应加入 .gitignore，真实 key 绝不进版本库。
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	localPath := localPathFor(path)
	if localData, err := os.ReadFile(localPath); err == nil {
		var local Config
		if err := yaml.Unmarshal(localData, &local); err != nil {
			return cfg, fmt.Errorf("解析本地覆盖文件 %s 失败: %w", localPath, err)
		}
		for k, v := range local.APIKeys {
			if cfg.APIKeys == nil {
				cfg.APIKeys = map[string]string{}
			}
			cfg.APIKeys[k] = v
		}
	}

	if cfg.Provider == "" {
		cfg.Provider = "deepseek"
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = "qwen"
	}
	return cfg, nil
}

// localPathFor 把 config.yaml 映射为同目录的 config.local.yaml。
func localPathFor(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".local" + ext
}

// Chat 解析聊天端点：返回端点、API key（环境变量优先）、模型名（可覆盖）。
func (c Config) Chat() (Endpoint, string, string, error) {
	ep, ok := endpoints[c.Provider]
	if !ok || ep.DefaultChatModel == "" {
		return Endpoint{}, "", "", fmt.Errorf("未知聊天 provider %q（可选: deepseek | glm | qwen）", c.Provider)
	}
	key, err := c.resolveKey(ep.APIKeyEnv)
	if err != nil {
		return Endpoint{}, "", "", err
	}
	model := c.ChatModel
	if model == "" {
		model = ep.DefaultChatModel
	}
	return ep, key, model, nil
}

// Embedding 解析 embedding 端点（deepseek 无此能力，需另选一家）。
func (c Config) Embedding() (Endpoint, string, string, error) {
	ep, ok := endpoints[c.EmbeddingProvider]
	if !ok || ep.DefaultEmbeddingModel == "" {
		return Endpoint{}, "", "", fmt.Errorf("未知 embedding provider %q（可选: qwen | glm | siliconflow；deepseek 无 embedding 接口）", c.EmbeddingProvider)
	}
	key, err := c.resolveKey(ep.APIKeyEnv)
	if err != nil {
		return Endpoint{}, "", "", err
	}
	model := c.EmbeddingModel
	if model == "" {
		model = ep.DefaultEmbeddingModel
	}
	return ep, key, model, nil
}

// resolveKey 按 env > 本地文件 的顺序解析一个 API key。
func (c Config) resolveKey(envName string) (string, error) {
	if v := os.Getenv(envName); v != "" {
		return v, nil
	}
	if v := c.APIKeys[envName]; v != "" {
		return v, nil
	}
	return "", fmt.Errorf("缺少 %s 的 API key：请设置环境变量 %s，或在 configs/config.local.yaml（已 gitignore）中配置 api_keys.%s", envName, envName, envName)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/config/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 对齐两个 configs 文件**

`configs/config.yaml` 整体替换为：

```yaml
# 聊天 provider：deepseek | glm | qwen
# DeepSeek 没有 /embeddings 接口，embedding 单独配一家：qwen | glm | siliconflow
provider: glm
embedding_provider: qwen

# 可选：覆盖默认模型名（默认见 internal/config/config.go）
# chat_model: glm-4.6
# embedding_model: text-embedding-v4
```

`configs/config.local.yaml.example` 整体替换为：

```yaml
# 复制为 config.local.yaml 后填入真实 key（该文件已 gitignore，绝不提交）。
# 环境变量始终优先于此文件。
api_keys:
  # ZHIPU_API_KEY: sk-...
  # DEEPSEEK_API_KEY: sk-...
  # DASHSCOPE_API_KEY: sk-...
  # SILICONFLOW_API_KEY: sk-...
```

注意：**不要动 `configs/config.local.yaml`**（用户本地真实 key）。

- [ ] **Step 6: gofmt + vet + 提交**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/config configs/config.yaml configs/config.local.yaml.example
git commit -m "feat: internal/config——端点表+配置加载（env 优先于本地覆盖文件）"
```

---

### Task 3: cmd/01-mcp——MCP 服务器与客户端

**Files:**
- Create: `cmd/01-mcp/main.go`、`cmd/01-mcp/expr.go`、`cmd/01-mcp/main_test.go`、`cmd/01-mcp/expr_test.go`、`docs/01-mcp.md`

**Interfaces:**
- Consumes: 无（不需要 API key，不依赖 internal/config）
- Produces（Task 6 依赖）:
  - 子进程拉起方式：`client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp", "serve")`
  - 工具名：`get_time(timezone)`、`calculate(expression)`；calculate 的错误以 `IsError=true` 的文本结果返回

- [ ] **Step 1: 写失败的测试**

`cmd/01-mcp/expr_test.go`：

```go
package main

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
		got, err := evaluate(c.input)
		if err != nil {
			t.Errorf("evaluate(%q) 不应报错: %v", c.input, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("evaluate(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestEvaluateInvalid(t *testing.T) {
	for _, input := range []string{
		"",      // 空
		"1+",    // 缺右操作数
		"(1+2",  // 缺右括号
		"3$5",   // 非法字符
		"1/0",   // 除零——错误是数据，返回给模型而不是崩溃
		"2 3",   // 多余 token
		"1.2.3", // 非法数字
	} {
		if _, err := evaluate(input); err == nil {
			t.Errorf("evaluate(%q) 应报错", input)
		}
	}
}
```

`cmd/01-mcp/main_test.go`：

```go
package main

import (
	"strings"
	"testing"
)

func TestTimeIn(t *testing.T) {
	s, err := timeIn("Asia/Shanghai")
	if err != nil {
		t.Fatalf("合法时区不应报错: %v", err)
	}
	if !strings.Contains(s, "CST") {
		t.Errorf("Asia/Shanghai 应渲染 CST 缩写, got %q", s)
	}
	if _, err := timeIn("Mars/Phobos"); err == nil {
		t.Error("非法时区应报错")
	}
	if _, err := timeIn(""); err != nil {
		t.Errorf("空时区应回退本地时区: %v", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/01-mcp/`
Expected: FAIL（`evaluate`、`timeIn` 未定义，编译错误）

- [ ] **Step 3: 写求值器 expr.go**

`cmd/01-mcp/expr.go`——四则运算的词法分析 + 递归下降解析（绝不 eval，非法输入只会得到明确报错）：

```go
// expr.go：安全的四则运算求值器。
// 手写 tokenize + 递归下降解析——非法输入只会得到明确报错，不可能执行任意代码。
package main

import (
	"fmt"
	"strconv"
	"unicode"
)

// evaluate 计算表达式：支持 + - * /、括号、一元负号、整数与小数。
func evaluate(input string) (float64, error) {
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
			start, dots := i, 0
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
//
//	expr   = term (('+'|'-') term)*
//	term   = factor (('*'|'/') factor)*
//	factor = NUMBER | '(' expr ')' | '-' factor

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

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./cmd/01-mcp/ -v`
Expected: TestTimeIn FAIL（timeIn 还没写——正常，main.go 在下一步），TestEvaluate* PASS

- [ ] **Step 5: 写 main.go（serve + call）**

`cmd/01-mcp/main.go`：

```go
// demo 01：MCP 服务器 + 客户端（stdio 传输）。不需要任何 API key。
//
// MCP 的核心思想：把"工具"放进独立进程，双方在 stdin/stdout 上用 JSON-RPC 对话：
// initialize 握手 → tools/list 发现工具 → tools/call 调用工具。
//
// 用法：
//
//	go run ./cmd/01-mcp serve   # 启动服务器（读 stdin / 写 stdout）
//	go run ./cmd/01-mcp call    # 拉起 serve 子进程，完整走一遍客户端流程
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // Windows 没有系统时区库，内嵌 tzdata 保证 LoadLocation 可用

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve()
	case "call":
		call()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: go run ./cmd/01-mcp <serve|call>")
}

// ── serve：MCP 服务器 ──────────────────────────────────────────────

func serve() {
	s := server.NewMCPServer("demo-tools", "1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	registerGetTime(s)
	registerCalculate(s)
	// ServeStdio 阻塞运行：stdin 收 JSON-RPC 请求，stdout 写响应。
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, "服务器退出:", err)
		os.Exit(1)
	}
}

// get_time：当前时间，可选 IANA 时区。
func registerGetTime(s *server.MCPServer) {
	tool := mcp.NewTool("get_time",
		mcp.WithDescription("获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。"),
		mcp.WithString("timezone",
			mcp.Description("IANA 时区名"),
			mcp.Enum("Asia/Shanghai", "UTC", "America/New_York", "Europe/London", "Asia/Tokyo"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := timeIn(req.GetString("timezone", ""))
		if err != nil {
			// 工具级错误：IsError=true 的正常响应，错误文本交给对方，连接不断
			return mcp.NewToolResultErrorf("%v", err), nil
		}
		return mcp.NewToolResultText(text), nil
	})
}

// timeIn 纯函数（便于单测）：把"现在"格式化到指定时区。
func timeIn(tz string) (string, error) {
	loc := time.Local
	if tz != "" {
		var err error
		if loc, err = time.LoadLocation(tz); err != nil {
			return "", fmt.Errorf("无法识别时区 %q", tz)
		}
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%s（%s，星期%s）",
		now.Format("2006-01-02 15:04:05"), now.Format("MST"),
		string([]rune("日一二三四五六")[int(now.Weekday())])), nil
}

// calculate：四则运算（求值器见 expr.go）。
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
		val, err := evaluate(raw)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("表达式无法计算", err), nil
		}
		// 去尾零：9.000000 → 9，2.500000 → 2.5
		text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
		return mcp.NewToolResultText(text), nil
	})
}

// ── call：MCP 客户端演示 ──────────────────────────────────────────

func call() {
	ctx := context.Background()
	// 拉起自己的 serve 作为子进程：stdio 传输 = 子进程的 stdin/stdout 就是信道
	c, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp", "serve")
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		log.Fatalf("启动子进程失败: %v", err)
	}

	// 1. initialize 握手：交换协议版本与能力
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "01-mcp-call", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		log.Fatalf("initialize 失败: %v", err)
	}

	// 2. tools/list：发现服务器有哪些工具
	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("tools/list 失败: %v", err)
	}
	for _, t := range res.Tools {
		fmt.Printf("发现工具: %s — %s\n", t.Name, t.Description)
	}

	// 3. tools/call：正常调用、错误作为数据返回（连接不会断）
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"get_time", map[string]any{"timezone": "Asia/Shanghai"}},
		{"calculate", map[string]any{"expression": "(1+2)*3"}},
		{"calculate", map[string]any{"expression": "1/0"}}, // 错误即数据
	} {
		callRes, err := c.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: tc.name, Arguments: tc.args},
		})
		if err != nil {
			log.Fatalf("tools/call %s 失败: %v", tc.name, err)
		}
		var sb strings.Builder
		for _, content := range callRes.Content {
			if txt, ok := content.(mcp.TextContent); ok {
				sb.WriteString(txt.Text)
			}
		}
		mark := "结果"
		if callRes.IsError {
			mark = "工具级错误（注意：连接没断，错误只是数据）"
		}
		fmt.Printf("调用 %s(%v)\n  → %s: %s\n", tc.name, tc.args, mark, sb.String())
	}
}
```

- [ ] **Step 6: 跑测试与手工验证**

```bash
go test ./cmd/01-mcp/ -v
```
Expected: 全部 PASS

```bash
go run ./cmd/01-mcp call
```
Expected: 打印发现的 2 个工具；三次调用输出时间、`9`、以及 `表达式无法计算: 除数为零`（带"工具级错误"标注）

printf 管道探针（不经 call，直接对 server 说 JSON-RPC）：

```bash
printf '%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp serve
```
Expected: stdout 出现 initialize 响应与 `"9"` 结果

- [ ] **Step 7: 写原理文档**

`docs/01-mcp.md`：

```markdown
# demo 01：MCP——把工具放进独立进程

## 这一步演示什么

MCP（Model Context Protocol）定义了"工具服务器"与"工具客户端"之间的对话方式。
本 demo 用 mcp-go 实现两端：

- `serve`：stdio 服务器，暴露 `get_time(timezone)` 与 `calculate(expression)` 两个工具
- `call`：拉起 `serve` 子进程，走完 initialize 握手 → tools/list 发现 → tools/call 调用

## 核心概念

1. **stdio 传输**：子进程的 stdin/stdout 就是 JSON-RPC 信道，没有端口、没有 HTTP。
2. **三个关键方法**：`initialize`（握手）、`tools/list`（发现）、`tools/call`（调用）。
3. **工具带 JSON Schema**：参数的形状由 schema 描述，调用方不需要读代码就知道怎么传参。
4. **错误即数据**：`1/0` 不会断开连接，而是返回 `IsError=true` 的文本结果——
   这为 demo 04 里"工具错误回填给模型"打底。

## 读码路线

`main.go` 从上往下：`serve()` 注册两个工具 → `call()` 三步客户端流程。
`expr.go` 是四则运算求值器（词法分析 + 递归下降，绝非 eval）。

## 运行

    go run ./cmd/01-mcp call

不需要 API key。也可以用官方调试器连接 serve：
`npx @modelcontextprotocol/inspector go run ./cmd/01-mcp serve`

## 与下一步的关系

demo 02 把同样的两个工具直接写成 Go 函数（进程内），对比"工具在进程外（MCP）"
与"工具在进程内"的差别；demo 04 会重新拉起本步的 serve，把 MCP 工具接进 Agent。
```

- [ ] **Step 8: gofmt + vet + 提交**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/01-mcp docs/01-mcp.md
git commit -m "feat: demo01 MCP——mcp-go 实现 stdio server（get_time/calculate）+ client 子进程演示"
```

---

### Task 4: cmd/02-llm——纯 function calling

**Files:**
- Create: `cmd/02-llm/main.go`、`cmd/02-llm/loop_test.go`、`cmd/02-llm/expr.go`、`docs/02-llm.md`

**Interfaces:**
- Consumes: `config.Load` / `Config.Chat()`（Task 2）
- Produces（Task 6 复制改造）:
  - `type toolCall struct { ID, Name, Arguments string }`
  - `type completeFn func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error)`
  - `runTurn(ctx, complete, tools, handlers map[string]func(map[string]any) string, history, user) (answer string, updated []openai.ChatCompletionMessageParamUnion, err error)`——最多 8 步，超限报"已达最大步数 8"
  - `dispatch(handlers, toolCall) string`——未知工具/坏 JSON/空参数都返回 `ERROR: ...` 文本
  - `assistantWithToolCalls(content string, calls []toolCall) openai.ChatCompletionMessageParamUnion`

- [ ] **Step 1: 写失败的测试**

`cmd/02-llm/loop_test.go`：

```go
package main

import (
	"context"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

// fakeComplete 用脚本回放模型行为：第 1 次要工具，第 2 次给最终回答。
func TestRunTurnToolLoop(t *testing.T) {
	var gotName string
	handlers := map[string]func(map[string]any) string{
		"echo": func(args map[string]any) string {
			gotName, _ = args["name"].(string)
			return "hi"
		},
	}
	calls := 0
	complete := func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		calls++
		if calls == 1 {
			return "", []toolCall{{ID: "c1", Name: "echo", Arguments: `{"name":"世界"}`}}, nil
		}
		return "完成", nil, nil
	}
	answer, updated, err := runTurn(context.Background(), complete, nil, handlers, nil, "打招呼")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "完成" {
		t.Fatalf("answer = %q", answer)
	}
	if gotName != "世界" {
		t.Errorf("工具收到的参数 name = %q", gotName)
	}
	if calls != 2 {
		t.Errorf("complete 调用 %d 次, want 2", calls)
	}
	// 更新后的历史应含：user、assistant(tool_calls)、tool、assistant(final) 共 4 条
	if len(updated) != 4 {
		t.Errorf("updated 应有 4 条消息, got %d", len(updated))
	}
}

// 模型永远要工具时必须在步数上限处停下，不能死循环。
func TestRunTurnMaxSteps(t *testing.T) {
	complete := func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		return "", []toolCall{{ID: "c", Name: "echo", Arguments: "{}"}}, nil
	}
	handlers := map[string]func(map[string]any) string{
		"echo": func(args map[string]any) string { return "x" },
	}
	_, _, err := runTurn(context.Background(), complete, nil, handlers, nil, "hi")
	if err == nil || !strings.Contains(err.Error(), "8") {
		t.Fatalf("应在 8 步上限处报错, got %v", err)
	}
}

func TestDispatch(t *testing.T) {
	handlers := map[string]func(map[string]any) string{
		"ok": func(args map[string]any) string { return "fine" },
	}
	if got := dispatch(handlers, toolCall{Name: "ok", Arguments: `{}`}); got != "fine" {
		t.Errorf("正常分发 = %q", got)
	}
	if got := dispatch(handlers, toolCall{Name: "nope", Arguments: `{}`}); !strings.HasPrefix(got, "ERROR:") {
		t.Errorf("未知工具应返回 ERROR: 前缀, got %q", got)
	}
	if got := dispatch(handlers, toolCall{Name: "ok", Arguments: `{bad`}); !strings.HasPrefix(got, "ERROR:") {
		t.Errorf("坏 JSON 应返回 ERROR: 前缀, got %q", got)
	}
	// 空参数字符串：视为无参数，不报 JSON 错
	if got := dispatch(handlers, toolCall{Name: "ok", Arguments: ""}); got != "fine" {
		t.Errorf("空 arguments 应按空参数处理, got %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/02-llm/`
Expected: FAIL（`runTurn`、`dispatch` 未定义，编译错误）

- [ ] **Step 3: 复制求值器与 timeIn**

把 `cmd/01-mcp/expr.go` 原样复制到 `cmd/02-llm/expr.go`，仅把文件头注释改为：

```go
// expr.go：安全的四则运算求值器（复制自 demo01——教学项目宁复制，不抽象）。
// 手写 tokenize + 递归下降解析——非法输入只会得到明确报错，不可能执行任意代码。
package main
```

- [ ] **Step 4: 写 main.go**

`cmd/02-llm/main.go`：

```go
// demo 02：纯 LLM function calling——工具就是进程内的普通 Go 函数。
//
// 对比 demo01：MCP 把工具放进另一个进程、用 JSON-RPC 调用；
// 这里没有协议层，工具注册为 Go func，由模型通过 tool_calls 驱动。
// runTurn 的"模型要工具 → 执行 → 结果回填 → 再问模型"循环是所有 Agent 的心脏。
//
// 运行：go run ./cmd/02-llm   （需要聊天 API key，见 README）
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"example-go-agent-lab/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)

	// openai-go 客户端：WithBaseURL 一行切换 DeepSeek/GLM/Qwen 等 OpenAI 兼容服务
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	complete := completeFnOf(sc, model)

	tools := []openai.ChatCompletionToolUnionParam{timeTool(), calcTool()}
	handlers := map[string]func(map[string]any) string{
		"get_time":  handleGetTime,
		"calculate": handleCalculate,
	}

	fmt.Println("LLM 工具循环已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []openai.ChatCompletionMessageParamUnion
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
		answer, updated, err := runTurn(context.Background(), complete, tools, handlers, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

// completeFnOf 把 openai-go 的调用包成 completeFn：
// 返回回答文本与精简后的 tool_calls（finish_reason=tool_calls 时非空）。
func completeFnOf(sc openai.Client, model string) completeFn {
	return func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		params := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(model),
			Messages: msgs,
		}
		if len(tools) > 0 {
			params.Tools = tools
		}
		resp, err := sc.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", nil, err
		}
		if len(resp.Choices) == 0 {
			return "", nil, fmt.Errorf("模型未返回任何候选回复")
		}
		var calls []toolCall
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			if tc.Type == "function" {
				calls = append(calls, toolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
			}
		}
		return resp.Choices[0].Message.Content, calls, nil
	}
}

// ── 工具循环（Agent 的心脏）───────────────────────────────────────

// toolCall 是模型返回的一次工具调用请求。
type toolCall struct {
	ID        string
	Name      string
	Arguments string // JSON 字符串
}

type completeFn func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error)

// runTurn 处理一轮用户输入：模型要工具就执行并回填，直到模型给出纯文本回答。
// history 不含 system（system 每次由调用方拼在最前）。
func runTurn(ctx context.Context, complete completeFn, tools []openai.ChatCompletionToolUnionParam, handlers map[string]func(map[string]any) string, history []openai.ChatCompletionMessageParamUnion, user string) (string, []openai.ChatCompletionMessageParamUnion, error) {
	msgs := append(history, openai.UserMessage(user))
	for step := 0; step < 8; step++ {
		content, calls, err := complete(ctx, msgs, tools)
		if err != nil {
			return "", history, err
		}
		if len(calls) == 0 { // 模型不再要工具 = 最终回答
			msgs = append(msgs, openai.AssistantMessage(content))
			return content, msgs, nil
		}
		// 模型决定调工具：assistant(tool_calls) 与每条结果都要写回消息流
		msgs = append(msgs, assistantWithToolCalls(content, calls))
		for _, c := range calls {
			result := dispatch(handlers, c)
			fmt.Printf("  ⚙ %s(%s)\n  ← %s\n", c.Name, c.Arguments, firstLine(result))
			msgs = append(msgs, openai.ToolMessage(result, c.ID))
		}
	}
	return "", history, fmt.Errorf("已达最大步数 8，模型仍在要求调用工具")
}

// dispatch 执行单个工具调用；一切错误都变成 ERROR: 文本回填给模型。
func dispatch(handlers map[string]func(map[string]any) string, c toolCall) string {
	h, ok := handlers[c.Name]
	if !ok {
		return fmt.Sprintf("ERROR: 未知工具 %q", c.Name)
	}
	var args map[string]any
	if strings.TrimSpace(c.Arguments) != "" {
		if err := json.Unmarshal([]byte(c.Arguments), &args); err != nil {
			return fmt.Sprintf("ERROR: 工具参数不是合法 JSON: %v", err)
		}
	}
	return h(args)
}

// assistantWithToolCalls 构造"带 tool_calls 的 assistant 消息"写回历史。
func assistantWithToolCalls(content string, calls []toolCall) openai.ChatCompletionMessageParamUnion {
	tc := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, c := range calls {
		tc = append(tc, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: c.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      c.Name,
					Arguments: c.Arguments,
				},
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(content),
			},
			ToolCalls: tc,
		},
	}
}

// ── 两个进程内工具 ────────────────────────────────────────────────

// fnTool 小助手：拼一个 function calling 工具定义。
func fnTool(name, desc string, props shared.FunctionParameters, required ...string) openai.ChatCompletionToolUnionParam {
	if len(required) > 0 {
		props["required"] = required
	}
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        name,
		Description: openai.String(desc),
		Parameters:  props,
	})
}

func timeTool() openai.ChatCompletionToolUnionParam {
	return fnTool("get_time",
		"获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。",
		shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"timezone": shared.FunctionParameters{"type": "string", "description": "IANA 时区名"},
			},
		})
}

func calcTool() openai.ChatCompletionToolUnionParam {
	return fnTool("calculate",
		"计算四则运算表达式的值，支持 + - * /、括号与一元负号。",
		shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"expression": shared.FunctionParameters{"type": "string", "description": "算术表达式，如 1+2*3"},
			},
		}, "expression")
}

// 工具的错误也返回字符串——错误是回填给模型的数据，不是异常。
// （timeIn 复制自 demo01——教学项目宁复制，不抽象）
func timeIn(tz string) (string, error) {
	loc := time.Local
	if tz != "" {
		var err error
		if loc, err = time.LoadLocation(tz); err != nil {
			return "", fmt.Errorf("无法识别时区 %q", tz)
		}
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%s（%s，星期%s）",
		now.Format("2006-01-02 15:04:05"), now.Format("MST"),
		string([]rune("日一二三四五六")[int(now.Weekday())])), nil
}

func handleGetTime(args map[string]any) string {
	tz, _ := args["timezone"].(string)
	text, err := timeIn(tz)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return text
}

func handleCalculate(args map[string]any) string {
	exprStr, _ := args["expression"].(string)
	val, err := evaluate(exprStr)
	if err != nil {
		return "ERROR: 表达式无法计算: " + err.Error()
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
}

// firstLine 取结果首行（超 120 rune 截断），让 REPL 轨迹可读。
func firstLine(s string) string {
	line := strings.SplitN(s, "\n", 2)[0]
	if r := []rune(line); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return line
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./cmd/02-llm/ -v`
Expected: 全部 PASS（runTurn 测试会向 stdout 打印 ⚙ 轨迹，属正常）

- [ ] **Step 6: 真实冒烟（需要聊天 key；本机已配 glm）**

```bash
printf '现在几点了？北京时间\ncalculate 1/0 会怎样\nexit\n' | go run ./cmd/02-llm
```
Expected: 看到 `⚙ get_time(...)` 与 `ERROR: 表达式无法计算: 除数为零` 的回填轨迹，以及模型的最终中文回答

- [ ] **Step 7: 写原理文档**

`docs/02-llm.md`：

```markdown
# demo 02：LLM function calling——工具循环

## 这一步演示什么

不给模型任何协议层，把 `get_time` / `calculate` 直接注册为进程内 Go 函数，
让模型通过 function calling 自主决定何时调用。核心是 `runTurn` 工具循环：

    模型返回 tool_calls → 执行工具 → 结果以 tool 消息回填 → 再问模型
    → ……直到模型给出纯文本回答

这个循环就是所有 Agent 的心脏；demo 04 只是给它换上更多工具。

## 核心概念

1. **tools 参数**：每个工具 = 名字 + 描述 + JSON Schema 参数（`fnTool`）。
2. **tool_calls 响应**：模型不直接执行任何东西，它只是"请求"调用；
   执行永远发生在我们的代码里。
3. **回填必须成对**：assistant(tool_calls) 消息与 tool 结果消息都要写回历史，
   `tool_call_id` 要对上，否则 API 报 400。
4. **错误是数据**：`1/0` 的结果是 `ERROR: 表达式无法计算: 除数为零` 文本，
   模型读到后会向用户如实转述——错误回填，而非崩溃。

## 读码路线

`main.go` 从上往下：`completeFnOf`（API 封装）→ `runTurn`/`dispatch`（循环）
→ `timeTool`/`calcTool`（工具定义）→ `handleGetTime`/`handleCalculate`（工具实现）。

## 运行

    go run ./cmd/02-llm

需要聊天 API key（见 README）。试试问"现在几点了？北京时间"和"calculate 1/0 会怎样"。

## 与前后步的关系

demo 01 的工具在另一个进程（MCP），这里的工具在本进程内——对照着读，
就能看清 MCP 到底解决了什么（进程/语言边界、独立升级、复用）。
demo 04 会把这两种来源的工具装进同一个循环。
```

- [ ] **Step 8: gofmt + vet + 提交**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/02-llm docs/02-llm.md
git commit -m "feat: demo02 LLM——openai-go function calling 工具循环（进程内 Go 工具）"
```

---

### Task 5: cmd/03-rag——检索增强问答

**Files:**
- Create: `cmd/03-rag/main.go`、`cmd/03-rag/chunk_test.go`、`docs/03-rag.md`

**Interfaces:**
- Consumes: `config.Load` / `Config.Chat()` / `Config.Embedding()`（Task 2）
- Produces（Task 6 复制）:
  - 向量库位置与集合名：`.chromem/` 目录、集合 `"notes"`（Task 6 的 search_notes 查同一个库）
  - `retrieve(ctx context.Context, col *chromem.Collection, query string) ([]chromem.Result, error)`——内部 `min(topK, col.Count())` 钳制，空库返回 `(nil, nil)`
  - `embedFn(sc openai.Client, model string) chromem.EmbeddingFunc`
  - `chunk(text string, size, overlap int) []string`

- [ ] **Step 1: 写失败的测试**

`cmd/03-rag/chunk_test.go`：

```go
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/philippgille/chromem-go"
)

func TestChunk(t *testing.T) {
	if got := chunk("", 500, 80); len(got) != 0 {
		t.Errorf("空文本应无块, got %d", len(got))
	}
	if got := chunk("hello", 500, 80); len(got) != 1 || got[0] != "hello" {
		t.Errorf("短文本应整块返回: %v", got)
	}
	got := chunk(strings.Repeat("字", 1200), 500, 80)
	if len(got) != 3 { // 步长 420：起点 0/420/840
		t.Fatalf("got %d 块, want 3", len(got))
	}
	for i, c := range got {
		if r := len([]rune(c)); r > 500 {
			t.Errorf("块 %d 超长: %d rune", i, r)
		}
	}
	// 相邻块应重叠 80 rune：块 2 的前 80 字 == 块 1 的后 80 字
	r0, r1 := []rune(got[0]), []rune(got[1])
	if string(r1[:80]) != string(r0[len(r0)-80:]) {
		t.Error("相邻块应重叠 80 rune")
	}
}

// 空库检索必须安静地返回空，不能触发 chromem 的 nResults>Count 报错。
func TestRetrieveEmptyCollection(t *testing.T) {
	db := chromem.NewDB() // 内存库，测试不碰磁盘
	col, err := db.GetOrCreateCollection("notes", nil, fakeEmbed)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := retrieve(context.Background(), col, "任意问题")
	if err != nil {
		t.Fatalf("空库应返回空结果而非报错: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("空库应无结果, got %d", len(rs))
	}
}

// fakeEmbed 确定性假向量（按 rune 累加），测试不调真实 embedding API。
var fakeEmbed = chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
	v := make([]float32, 8)
	for i, r := range text {
		v[i%8] += float32(r)
	}
	return v, nil
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/03-rag/`
Expected: FAIL（`chunk`、`retrieve` 未定义，编译错误）

- [ ] **Step 3: 写 main.go**

`cmd/03-rag/main.go`：

```go
// demo 03：RAG——检索增强问答。
//
//	ingest：docs/notes/*.md → 切块 → embedding → .chromem/（chromem-go 持久化）
//	ask：   问题 → 向量检索 top-k → 拼进 prompt → LLM 只依据资料作答并标注引用
//
// 运行：
//
//	go run ./cmd/03-rag ingest
//	go run ./cmd/03-rag ask "MCP 是什么"
//
// 注意：切换 embedding_provider/model 后旧向量库不兼容，需 `rm -rf .chromem` 重建。
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

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/config"
)

const (
	chunkSize = 500 // 每块最大 rune 数
	overlap   = 80  // 相邻块重叠 rune 数：防止关键句正好被块边界切断
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

	// embedding 与聊天可以是两家：deepseek 没有 /embeddings
	eep, ekey, emodel, err := cfg.Embedding()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)
	esc := openai.NewClient(option.WithAPIKey(ekey), option.WithBaseURL(eep.BaseURL))

	db, err := chromem.NewPersistentDB(".chromem", false)
	if err != nil {
		log.Fatalf("打开向量库失败: %v", err)
	}
	col, err := db.GetOrCreateCollection("notes", nil, embedFn(esc, emodel))
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
		log.Fatal(`ask 需要跟一个问题，例如: go run ./cmd/03-rag ask "MCP 是什么"`)
	}
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	if err := ask(ctx, sc, model, col, question); err != nil {
		log.Fatalf("ask 失败: %v", err)
	}
}

// chunk：固定 rune 窗口 + 重叠切块。按 rune 计数保证中文不被从字节中间切断。
func chunk(text string, size, overlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	step := size - overlap
	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := min(start+size, len(runes))
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

// embedFn 把 openai-go 的 /embeddings 调用适配成 chromem 要的 EmbeddingFunc。
func embedFn(sc openai.Client, model string) chromem.EmbeddingFunc {
	return chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
		resp, err := sc.Embeddings.New(ctx, openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: []string{text}},
			Model: openai.EmbeddingModel(model),
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("embedding 接口未返回向量")
		}
		v := make([]float32, len(resp.Data[0].Embedding))
		for i, f := range resp.Data[0].Embedding {
			v[i] = float32(f)
		}
		return v, nil
	})
}

// retrieve 检索 top-k 相关块。chromem 要求 1 <= n <= 库内文档数，
// 所以先钳制；空库安静返回空，由调用方给"未命中"提示。
func retrieve(ctx context.Context, col *chromem.Collection, query string) ([]chromem.Result, error) {
	n := min(topK, col.Count())
	if n == 0 {
		return nil, nil
	}
	return col.Query(ctx, query, n, nil, nil)
}

// ingest 建库：ID 取 "文件名#序号"，重复 ingest 按 ID 覆盖（幂等）。
func ingest(ctx context.Context, col *chromem.Collection) error {
	files, _ := filepath.Glob("docs/notes/*.md")
	var docs []chromem.Document
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", f, err)
		}
		base := filepath.Base(f)
		chunks := chunk(string(data), chunkSize, overlap)
		for i, ch := range chunks {
			docs = append(docs, chromem.Document{
				ID:       fmt.Sprintf("%s#%04d", base, i),
				Content:  ch,
				Metadata: map[string]string{"source": base},
			})
		}
		log.Printf("已切分 %s: %d 块", base, len(chunks))
	}
	if len(docs) == 0 {
		return fmt.Errorf("docs/notes/ 下没有找到 .md 语料")
	}
	if err := col.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		return fmt.Errorf("写入向量库: %w", err)
	}
	log.Printf("入库完成：共 %d 块（已持久化到 .chromem/）", len(docs))
	return nil
}

// ask 检索 + 生成：system prompt 约束"只依据资料作答"，回答标注 [n] 引用。
func ask(ctx context.Context, sc openai.Client, model string, col *chromem.Collection, question string) error {
	results, err := retrieve(ctx, col, question)
	if err != nil {
		return fmt.Errorf("检索失败: %w", err)
	}
	if len(results) == 0 {
		fmt.Println("未命中任何资料。请先运行: go run ./cmd/03-rag ingest")
		return nil
	}

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n", i+1, r.Metadata["source"], r.Similarity, r.Content)
	}
	system := "你是知识库问答助手。只依据下面提供的资料回答问题；引用资料时标注编号（如 [1]）；" +
		"如果资料不足以回答，明确说明资料未覆盖，不要编造。\n\n资料：\n" + sb.String()

	resp, err := sc.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(question),
		},
	})
	if err != nil {
		return err
	}
	fmt.Printf("助手> %s\n\n命中块：\n", resp.Choices[0].Message.Content)
	for i, r := range results {
		fmt.Printf("  [%d] %s（相似度 %.3f）\n", i+1, r.Metadata["source"], r.Similarity)
	}
	return nil
}
```

注意 `ingest` 只对每篇笔记调用一次 `chunk`，结果存 `chunks` 变量复用（切块与日志共用）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./cmd/03-rag/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 真实冒烟（需要 embedding + 聊天 key；本机 glm + qwen）**

```bash
go run ./cmd/03-rag ingest
go run ./cmd/03-rag ask "MCP 是什么"
```
Expected: ingest 打印每篇笔记的切块数；ask 给出带 `[1]` 引用的回答与命中块列表。
若 embedding key 未配，跳过本步并在交付说明中注明（单测已覆盖纯逻辑）。

- [ ] **Step 6: 写原理文档**

`docs/03-rag.md`：

```markdown
# demo 03：RAG——检索增强问答

## 这一步演示什么

LLM 不知道你的私有资料，且会一本正经地编造。RAG 的解法分两步：

- `ingest`：把 `docs/notes/*.md` 切块、向量化、存进 chromem-go（嵌入式向量库）
- `ask`：把问题向量化，检索最相似的 top-k 块，拼进 prompt，
  要求模型只依据资料作答并标注引用

## 核心概念

1. **切块（chunk）**：embedding 模型对超长文本的表征会稀释；
   固定 rune 窗口 + 80 rune 重叠，防止关键句被块边界切断。
2. **向量检索**：文档与问题都变成同一空间的向量，用余弦相似度找近邻；
   chromem-go 是嵌入式库——无外部服务，一个目录就是库。
3. **引用式回答**：system prompt 约束"只依据资料、标注 [n]、不足就说未覆盖"，
   把"生成"锚定在"检索"上，压幻觉。
4. **空库钳制**：chromem 要求 `1 <= n <= 库内文档数`，所以检索前先
   `min(topK, Count())`；空库给"请先 ingest"而不是报错。

## 读码路线

`main.go` 从上往下：`chunk`（切块）→ `embedFn`（向量化适配）→
`retrieve`（检索+钳制）→ `ingest`（建库）→ `ask`（检索+生成）。

## 运行

    go run ./cmd/03-rag ingest
    go run ./cmd/03-rag ask "MCP 是什么"

需要 embedding 与聊天两类 API key（见 README）。切换 embedding 模型后
旧库不兼容：`rm -rf .chromem && go run ./cmd/03-rag ingest`。

## 与下一步的关系

本步的检索是"固定前置流程"：一问必检索。demo 04 把检索包装成
`search_notes` 工具，由模型自主决定何时检索——RAG 从流程变成了能力。
```

- [ ] **Step 7: gofmt + vet + 提交**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/03-rag docs/03-rag.md
git commit -m "feat: demo03 RAG——chromem-go 向量库 + ingest/ask + 引用式回答"
```

---

### Task 6: cmd/04-agent——综合智能体

**Files:**
- Create: `cmd/04-agent/main.go`、`cmd/04-agent/convert_test.go`、`docs/04-agent.md`

**Interfaces:**
- Consumes:
  - Task 2: `config.Load` / `Config.Chat()` / `Config.Embedding()`
  - Task 3: `go run ./cmd/01-mcp serve` 子进程 + 工具 `get_time`/`calculate`
  - Task 5: `.chromem/` 集合 `"notes"`、`retrieve`、`embedFn`（复制）
  - Task 4: `runTurn`/`dispatch`/`assistantWithToolCalls`/`completeFn`/`toolCall`/`firstLine`（复制）
- Produces: 最终 demo；`mcpToolsToOpenAI(tools []mcp.Tool) []openai.ChatCompletionToolUnionParam`

- [ ] **Step 1: 写失败的测试**

`cmd/04-agent/convert_test.go`：

```go
package main

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPToolsToOpenAI(t *testing.T) {
	tools := []mcp.Tool{{
		Name:        "get_time",
		Description: "获取当前时间",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"timezone": map[string]any{"type": "string"},
			},
			Required: []string{"timezone"},
		},
	}}
	got := mcpToolsToOpenAI(tools)
	if len(got) != 1 {
		t.Fatalf("got %d 个工具, want 1", len(got))
	}
	f := got[0].GetFunction()
	if f == nil {
		t.Fatal("应为 function 类型工具")
	}
	if f.Name != "get_time" {
		t.Errorf("name = %q", f.Name)
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

func TestMCPToolsToOpenAIEmptyRequired(t *testing.T) {
	tools := []mcp.Tool{{
		Name:        "ping",
		Description: "",
		InputSchema: mcp.ToolInputSchema{Type: "object", Properties: map[string]any{}},
	}}
	got := mcpToolsToOpenAI(tools)
	b, _ := json.Marshal(got[0].GetFunction().Parameters)
	var params map[string]any
	_ = json.Unmarshal(b, &params)
	if _, exists := params["required"]; exists {
		t.Errorf("无必填参数时不应输出 required 字段: %s", b)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/04-agent/`
Expected: FAIL（`mcpToolsToOpenAI` 未定义，编译错误）

- [ ] **Step 3: 写 main.go**

`cmd/04-agent/main.go`：

```go
// demo 04：综合智能体 = 02 的工具循环 + 01 的 MCP 工具 + 03 的 RAG 检索（工具化）。
//
// 三种能力装进同一个 runTurn 循环，模型自主决定调用哪个：
//   - get_time / calculate —— 来自 MCP 子进程（demo01 的 serve）
//   - search_notes ———— 本进程直连 chromem 向量库（demo03 建的库）
//
// 运行（先跑过 demo03 的 ingest）：
//
//	go run ./cmd/04-agent
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/config"
)

const topK = 3

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ctx := context.Background()

	// ── 聊天模型 ─────────────────────────────────────────────
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	complete := completeFnOf(sc, model)

	toolDefs := []openai.ChatCompletionToolUnionParam{}
	handlers := map[string]func(map[string]any) string{}

	// ── 工具来源一：MCP 子进程（demo01）──────────────────────
	mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp", "serve")
	if err != nil {
		log.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpc.Close()
	if err := mcpc.Start(ctx); err != nil {
		log.Fatalf("启动 MCP 子进程失败: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "04-agent", Version: "1.0.0"}
	if _, err := mcpc.Initialize(ctx, initReq); err != nil {
		log.Fatalf("MCP initialize 失败: %v", err)
	}
	toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("tools/list 失败: %v", err)
	}
	toolDefs = append(toolDefs, mcpToolsToOpenAI(toolsRes.Tools)...)
	for _, t := range toolsRes.Tools {
		log.Printf("注册 MCP 工具: %s", t.Name)
		handlers[t.Name] = func(ctx context.Context, args map[string]any) string {
			res, err := mcpc.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: t.Name, Arguments: args},
			})
			if err != nil {
				return "ERROR: " + err.Error()
			}
			var sb strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					sb.WriteString(tc.Text)
				}
			}
			if res.IsError { // 工具级错误也是数据
				return "ERROR: " + sb.String()
			}
			return sb.String()
		}
	}

	// ── 工具来源二：RAG 检索（demo03 建的库，本进程直连）─────
	eep, ekey, emodel, err := cfg.Embedding()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)
	esc := openai.NewClient(option.WithAPIKey(ekey), option.WithBaseURL(eep.BaseURL))
	db, err := chromem.NewPersistentDB(".chromem", false)
	if err != nil {
		log.Fatalf("打开向量库失败: %v", err)
	}
	col, err := db.GetOrCreateCollection("notes", nil, embedFn(esc, emodel))
	if err != nil {
		log.Fatalf("创建集合失败: %v", err)
	}
	toolDefs = append(toolDefs, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name: "search_notes",
		Description: openai.String(
			"在项目知识库（MCP 协议、RAG 基础、Agent 模式、Go 并发等中文笔记）中检索资料。" +
				"用户问题涉及这些主题时调用；闲聊或常识问题不必调用。"),
		Parameters: shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"query": shared.FunctionParameters{"type": "string", "description": "检索关键词或问题"},
			},
			"required": []string{"query"},
		},
	}))
	handlers["search_notes"] = func(ctx context.Context, args map[string]any) string {
		q, _ := args["query"].(string)
		if strings.TrimSpace(q) == "" {
			return "ERROR: 缺少 query 参数"
		}
		results, err := retrieve(ctx, col, q) // 空库由 retrieve 钳制，安静返回空
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if len(results) == 0 {
			return "未命中任何资料（知识库可能尚未 ingest，可提示用户先运行 go run ./cmd/03-rag ingest）。"
		}
		var sb strings.Builder
		for i, r := range results {
			fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n", i+1, r.Metadata["source"], r.Similarity, r.Content)
		}
		return sb.String()
	}
	log.Printf("注册本地工具: search_notes")

	// ── REPL：三种工具，一个循环 ─────────────────────────────
	fmt.Println("综合智能体已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []openai.ChatCompletionMessageParamUnion
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
		answer, updated, err := runTurn(ctx, complete, toolDefs, handlers, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

// mcpToolsToOpenAI：MCP 的 InputSchema（JSON Schema）平移为
// OpenAI function calling 的 tools 格式——纯函数，可单测。
func mcpToolsToOpenAI(tools []mcp.Tool) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
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
		fd := shared.FunctionDefinitionParam{Name: t.Name, Parameters: params}
		if t.Description != "" {
			fd.Description = openai.String(t.Description)
		}
		out = append(out, openai.ChatCompletionFunctionTool(fd))
	}
	return out
}

// ── 以下复制自 demo02 / demo03——教学项目宁复制，不抽象 ──────────
```

复制块（紧跟在上述注释后面写入 main.go，与本计划中的定义**逐字一致**，import 已含在文件头）：
`toolCall` 类型、`completeFn` 类型、`runTurn`、`dispatch`、`assistantWithToolCalls`、
`completeFnOf`、`firstLine`——逐字复制本计划 **Task 4 Step 4** 代码块中的同名定义；
`embedFn`、`retrieve`——逐字复制本计划 **Task 5 Step 3** 代码块中的同名定义
（`retrieve` 内的 `topK` 用本文件顶部的 `const topK = 3`）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./cmd/04-agent/ -v`
Expected: 两个转换测试 PASS；`go build ./...` 通过

- [ ] **Step 5: 真实冒烟（需要聊天 + embedding key，且已 ingest）**

```bash
printf '现在几点了？北京时间，然后查一下知识库里 MCP 是什么\nexit\n' | go run ./cmd/04-agent
```
Expected: 轨迹里同时出现 `⚙ get_time(...)` 与 `⚙ search_notes(...)`——
两种来源的工具被同一个循环调度；最后是模型的综合回答

- [ ] **Step 6: 写原理文档**

`docs/04-agent.md`：

```markdown
# demo 04：综合智能体——把一切装进工具循环

## 这一步演示什么

前四步的合体：`runTurn` 工具循环（demo02）驱动两种来源的工具——

- **MCP 工具**（demo01）：子进程里的 get_time / calculate，经
  initialize → tools/list 发现 → tools/call 调用
- **本地工具**（demo03）：search_notes 直连 chromem 向量库

模型自主决定调哪个、调几次；Agent = LLM + 工具 + 循环。

## 核心概念

1. **schema 转换**：MCP 的 InputSchema 与 OpenAI function calling 的
   parameters 都是 JSON Schema，`mcpToolsToOpenAI` 做纯平移。
2. **工具即能力，来源无关**：循环只认 `func(map[string]any) string`，
   工具在别的进程（MCP）还是本进程（RAG），模型与循环都不感知差别。
3. **RAG 从流程变能力**：demo03 里检索是必经前置；这里检索只是一个
   可选工具，模型判断需要才调。
4. **错误即数据（全链路）**：MCP 工具级错误（IsError）、求值器错误、
   检索未命中，一律变成 ERROR:/提示文本回填，循环永不因工具失败而崩。

## 读码路线

`main.go`：`main` 依序装配（聊天 → MCP 工具组 → RAG 工具组 → REPL）→
`mcpToolsToOpenAI`（转换）→ 文件尾部的复制块（来自 demo02/03）。

## 运行

    go run ./cmd/03-rag ingest   # 先建库（只需一次）
    go run ./cmd/04-agent

试试一句话同时触发两种工具："现在几点了？北京时间，然后查一下知识库里 MCP 是什么"。

## 回顾：四步拼图

| 步 | 学到 | 在这里变成 |
|---|---|---|
| 01 MCP | 工具可以跨进程、带 schema、错误即数据 | 一个工具来源 |
| 02 LLM | 工具循环：tool_calls → 执行 → 回填 | 循环本体 |
| 03 RAG | 切块、向量检索、引用式回答 | search_notes 工具 |
| 04 Agent | 装配：循环 + 多来源工具 + 自主决策 | 这个 demo |
```

- [ ] **Step 7: gofmt + vet + 提交**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/04-agent docs/04-agent.md
git commit -m "feat: demo04 综合智能体——MCP 工具 + RAG 工具装进统一循环"
```

---

### Task 7: README 重写与全量收尾

**Files:**
- Modify: `README.md`（整体重写）
- Check: `docs/notes/*.md`（清理对旧 demo 名的引用）

**Interfaces:**
- Consumes: Task 2-6 的全部产出
- Produces: 对外文档定稿

- [ ] **Step 1: 检查语料中的过时引用**

Run: `grep -n "01-mcp-server\|02-mcp-agent\|04-combo-agent\|internal/provider\|internal/llm" docs/notes/*.md || echo CLEAN`
Expected: 大概率 `CLEAN`（notes 是独立知识笔记）；若有命中，把对应句子改成
新结构（`01-mcp`、`02-llm`、`04-agent`、`internal/config`）

- [ ] **Step 2: 重写 README.md**

整体替换为：

```markdown
# example-go-agent-lab

用 Go 实现「Agent｜MCP+LLM+RAG｜智能体」的四步递进 demo——**简写教学版**。
每个概念独立成步、每个 demo 一个自包含的 main 包，从上往下读完一个文件即懂。
实现全部用官方/社区库（mcp-go、openai-go、chromem-go），但 internals 靠注释
与 `docs/` 讲清。

## 四步递进

| # | demo | 命令 | 演示什么 |
|---|------|------|----------|
| 01 | MCP | `go run ./cmd/01-mcp call` | stdio MCP server（get_time/calculate）+ client 子进程演示，不需要 key |
| 02 | LLM | `go run ./cmd/02-llm` | function calling 工具循环：工具是进程内 Go 函数 |
| 03 | RAG | `go run ./cmd/03-rag ingest` 然后 `go run ./cmd/03-rag ask "MCP 是什么"` | chromem-go 向量库 + 引用式问答 |
| 04 | Agent | `go run ./cmd/04-agent` | MCP 工具 + RAG 检索装进同一个工具循环，模型自主调度 |

每步原理见 `docs/0X-*.md`。递进关系：01 教"工具在进程外"（协议），
02 教"工具在进程内"（循环），03 教"检索增强"，04 把三者装配成智能体。

## API key（不进版本库）

key 两种来源，优先级：**环境变量 > 本地覆盖文件**。

| 变量 | 用途 |
|------|------|
| `DEEPSEEK_API_KEY` | 聊天（deepseek） |
| `ZHIPU_API_KEY` | 聊天/Embedding（glm） |
| `DASHSCOPE_API_KEY` | 聊天/Embedding（qwen） |
| `SILICONFLOW_API_KEY` | Embedding（siliconflow） |

方式一（环境变量）——PowerShell：`$env:ZHIPU_API_KEY="sk-..."`；
Git Bash：`export ZHIPU_API_KEY=sk-...`

方式二（本地文件）——`cp configs/config.local.yaml.example configs/config.local.yaml`
后填入真实 key。该文件已 gitignore（**切勿把 key 写进被跟踪的 config.yaml**）。

## Provider 切换

`configs/config.yaml`：

```yaml
provider: glm             # 聊天：deepseek | glm | qwen
embedding_provider: qwen  # 嵌入：qwen | glm | siliconflow（deepseek 无 embedding）
```

端点与默认模型见 `internal/config/config.go`（本项目唯一共享包）。
**切换 embedding 后**旧 `.chromem/` 与新向量空间不兼容：
`rm -rf .chromem && go run ./cmd/03-rag ingest`

## 快速验证（不需要 API key）

demo01 是纯本地 stdio 服务器，可直接喂 JSON-RPC：

```bash
printf '%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp serve
```

或用官方调试器：`npx @modelcontextprotocol/inspector go run ./cmd/01-mcp serve`

## 测试

```bash
go test ./...
```

各 demo 的纯逻辑（求值器、工具循环、schema 转换、切块）以 package main
就地单测；不对真实 API 做单测。

## 目录结构

```text
cmd/01-mcp…04-agent/  四个自包含 demo（含就地 _test.go）
internal/config/      唯一共享包：端点表 + key 解析（env > config.local.yaml）
docs/                 每步原理短文 + notes 知识库语料（RAG 用）
configs/              config.yaml + local 覆盖模板
.chromem/             向量库持久化（gitignore，ingest 后生成）
```

## 非目标

Web UI、流式输出、多 Agent 协作、独立向量数据库、多轮记忆管理。
```

- [ ] **Step 3: 全量验证**

```bash
gofmt -l . && go vet ./... && go test ./... && go build ./...
go run ./cmd/01-mcp call
```
Expected: gofmt 无输出、vet/test/build 全过、demo01 三次调用输出正常

- [ ] **Step 4: 提交**

```bash
git add README.md docs/notes
git commit -m "docs: 重写 README 对齐教学版四步结构"
```

---

## 计划自审记录

- **Spec 覆盖**：spec 的目录结构（Task 1-7 全部落位）、四步内容（Task 3-6）、
  config 三 provider + env 优先（Task 2）、测试就地（各 Task）、迁移分步提交
  （每 Task 一提交）、docs/notes 保留微修（Task 7）、README 重写（Task 7）——无遗漏。
- **占位符**：无 TBD/TODO；Task 6 的复制块指明"与 Task 4/5 逐字一致"并逐项列名。
- **类型一致性**：`runTurn`/`dispatch`/`completeFn`/`toolCall` 在 Task 4 定义、
  Task 6 逐字复制；`retrieve`/`embedFn` 在 Task 5 定义、Task 6 逐字复制；
  `Config.Chat()/Embedding()` 返回 `(Endpoint, string, string, error)` 三处消费一致。
- **Review Focus**：五项均有对应任务的具体测试（见各任务 Step 1）。

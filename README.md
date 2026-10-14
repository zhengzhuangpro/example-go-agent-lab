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

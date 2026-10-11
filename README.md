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

## API key（不进版本库）

key 有两种来源，优先级：**环境变量 > 本地覆盖文件**：

| 变量 | 用途 |
|------|------|
| `DEEPSEEK_API_KEY` | 聊天（deepseek） |
| `ZHIPU_API_KEY` | 聊天/Embedding（glm） |
| `DASHSCOPE_API_KEY` | 聊天/Embedding（qwen） |
| `SILICONFLOW_API_KEY` | Embedding（siliconflow） |

方式一（环境变量）——Windows PowerShell：`$env:DEEPSEEK_API_KEY="sk-..."`；Git Bash：`export DEEPSEEK_API_KEY=sk-...`

方式二（本地覆盖文件，免每次设环境变量）——把模板复制为 `configs/config.local.yaml` 后填入真实 key：

```bash
cp configs/config.local.yaml.example configs/config.local.yaml
```

`config.local.yaml` 已加入 `.gitignore`，永远不会被提交（**切勿把 key 写进 `configs/config.yaml`——那个文件是被跟踪的**）。环境变量始终优先于本地文件，CI/容器场景不受影响。

## Provider 切换

`configs/config.yaml`：

```yaml
provider: deepseek        # 聊天：deepseek | glm | qwen
embedding_provider: qwen  # 嵌入：qwen | glm | siliconflow（deepseek 无 embedding 接口）
# chat_model: deepseek-chat        # 可选：覆盖默认模型名
# embedding_model: text-embedding-v4
```

默认端点与模型见 `internal/provider/provider.go`。

注意：**切换 embedding_provider 或 embedding_model 后**，旧 `.chromem/` 向量库与新模型的向量空间不兼容，必须先删除再重建：

```bash
rm -rf .chromem && go run ./cmd/03-rag ingest
```

## 快速验证（不需要 API key）

demo01 是纯本地 stdio 服务器，可以手工喂 JSON-RPC 验证：

```bash
printf '%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp-server
```

也可以用官方调试器：`npx @modelcontextprotocol/inspector go run ./cmd/01-mcp-server`

## 预期输出示例（demo 02）

```text
你> 现在几点了？北京时间
  ⚙ get_time({"timezone":"Asia/Shanghai"})
  ← 2026-10-11 15:04:05（CST，星期日）
助手> 现在是北京时间 2026-10-11 15:04:05，星期日。
你> calculate 1/0 会怎样
  ⚙ calculate({"expression":"1/0"})
  ← ERROR: 表达式无法计算: 除数为零
助手: 计算器报告了除数为零……
```

（时间与模型措辞以实际运行为准；重点看：工具调用轨迹、错误作为工具结果回填而非崩溃。）

## 测试

```bash
go test ./...
```

纯逻辑（表达式求值、schema 转换、agent 循环、文本切分、配置加载）全部有单测；
不对真实 API 做单测。

## 目录结构

```text
cmd/          四个递进 demo
internal/     可复用组件（provider/llm/expr/mcp2llm/agent/splitter/config/appenv）
docs/         原理说明 + notes 知识库语料
configs/      config.yaml
.chromem/     向量库持久化（gitignore，ingest 后生成）
```

## 非目标

Web UI、流式输出、独立向量数据库、多 Agent 协作——这些主题由姊妹项目（手写实现）覆盖。

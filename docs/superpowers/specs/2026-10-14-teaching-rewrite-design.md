# example-go-agent-lab 教学版重写设计

日期：2026-10-14
状态：已与用户确认
前作：[2026-10-11-go-agent-lab-design.md](2026-10-11-go-agent-lab-design.md)（生产风格版，本次全部重写）

## 目标

把项目重写为**简写教学版**：用官方/社区库（mcp-go、openai-go、chromem-go）实现
「Agent｜MCP+LLM+RAG｜智能体」四步递进 demo，每个概念独立成步，代码精简到
最小可运行，一个 demo 从上往下读完一个文件即懂。

与旧版的区别：旧版 02 把 MCP 和 LLM 混在一步、internal/ 组件化较重；
新版每个概念（MCP / LLM / RAG / Agent）各占一步，教学优先于工程化。

## 已确认的关键决策

| 决策点 | 选择 |
|---|---|
| 四步划分 | 01-MCP（server+client）→ 02-LLM（纯 function calling）→ 03-RAG → 04-Agent 整合 |
| 重写程度 | 全部重写（cmd/ 与 internal/ 删除重写，docs 与 README 重写） |
| 实现风格 | 全部用库的最小实现；协议细节不手写，靠注释与文档讲清 internals |
| 代码组织 | 方案 A：每个 demo 自包含一个 main 包；仅共享 `internal/config` |
| 语言 | 代码注释与文档用中文 |

## 目录结构

```
example-go-agent-lab/
├── README.md                  # 重写：四步总览 + 运行方式
├── go.mod                     # 依赖不变（mcp-go / openai-go / chromem-go / yaml）
├── configs/
│   ├── config.yaml            # 沿用当前值（provider: glm）
│   └── config.local.yaml*     # 本地 key 覆盖机制沿用；现有文件不动（gitignored）
├── internal/
│   └── config/                # 唯一共享包：provider 端点表 + key 加载（env > local 文件）
├── cmd/
│   ├── 01-mcp/main.go         # 子命令 serve / call：MCP server + client（无需 API key）
│   ├── 02-llm/main.go         # openai-go function calling REPL（进程内 Go 工具）
│   ├── 03-rag/main.go         # chromem-go：ingest / ask 子命令
│   └── 04-agent/main.go       # 综合智能体：MCP 工具 + RAG 工具 + 统一循环
├── cmd/*/main_test.go         # package main 就地测各 demo 纯逻辑（见「测试」节）
└── docs/
    ├── 01-mcp.md … 04-agent.md  # 每步原理短文（重写，~80 行/篇）
    ├── notes/*.md               # RAG 语料，保留微修
    └── superpowers/             # 旧 spec/plan 保留为历史，新文档追加
```

## Demo 设计

### 01-MCP — MCP 服务器与客户端（~200 行）

- 库：`github.com/mark3labs/mcp-go`（server + client）
- `serve` 子命令：stdio MCP server，2 个工具：
  - `get_time(timezone)`：当前时间，时区可省略
  - `calculate(expression)`：四则运算，内联 ~60 行递归下降求值器；
    除零等错误作为工具结果文本返回（错误是数据不是崩溃）
- `call` 子命令：client 以子进程拉起 `go run ./cmd/01-mcp serve`，
  initialize → ListTools → CallTool 各演示一遍
- 无需 API key，可独立运行；04 会再次以子进程方式拉起它
- 教学点：JSON-RPC over stdio、工具 schema、错误即数据

### 02-LLM — 纯 function calling（~150 行）

- 库：`github.com/openai/openai-go/v3`
- 同样两个工具，但直接注册为**进程内** Go 函数（与 01 对比：
  工具在进程内 vs MCP 把工具放进程外）
- 手写 ~40 行工具循环：模型返回 tool_calls → 执行 → 以 tool role 回填 → 再请求，
  直到自然语言回答
- REPL：打印 ⚙ 工具调用轨迹 / ← 工具结果 / 助手回答
- 教学点：function calling 的请求/响应结构、工具循环是 Agent 的心脏

### 03-RAG — 检索增强问答（~150 行）

- 库：`github.com/philippgille/chromem-go` + OpenAI 兼容 /embeddings
- `ingest`：读 `docs/notes/*.md` → 内联 ~20 行切块（固定窗口+重叠）→ 入库
- `ask`：query 向量化 → top-k 检索 → 带来源引用的 prompt → LLM 回答
- 教学点：为什么切块、top-k、引用式回答压幻觉
- 依赖：需要 embedding + chat 的 API key；先 ingest 才能 ask

### 04-Agent — 综合智能体（~200 行）

- 拉起 01 的 server（子进程），MCP 工具 schema → openai tools 参数转换
  （内联 ~30 行纯函数）
- 本地再注册 `search_notes` 工具：查询 03 建好的向量库
- 统一 agent 循环：LLM 自主决定调用 MCP 工具或本地 RAG 工具 → 执行 →
  回填 → 循环；max steps 8 保护
- 教学点：Agent = LLM + 工具 + 循环；MCP 让工具来自任意进程；RAG 只是工具之一

## config / 依赖 / 测试

- **config**：保留多 provider 表（chat: deepseek/glm/qwen；
  embedding: qwen/glm/siliconflow），加载优先级 env > `configs/config.local.yaml`
  > `configs/config.yaml`；现有 config.local.yaml 不动
- **依赖**：go.mod 四个直接依赖不变，版本不主动升级
- **测试**：`go test ./...` 通过。internal/config 有单测；各 demo 的纯逻辑
  （01 求值器、03 切块、04 schema 转换、02 循环 fake 测试）以 package main
  就地 `_test.go` 覆盖；不对真实 API 做单测

## 迁移方式

直接在 main 分步提交：先删旧 cmd/ 与 internal/（保留 configs、docs/superpowers、
docs/notes），随后每步 demo 一个 commit，README/docs 最后收尾。旧 spec 与
plan 文档保留为历史。

## 非目标

Web UI、流式输出、多 Agent 协作、独立向量数据库、多轮记忆管理——不在此范围。

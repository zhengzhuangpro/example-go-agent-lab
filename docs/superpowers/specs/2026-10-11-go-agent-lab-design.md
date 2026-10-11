# example-go-agent-lab 设计文档

日期：2026-10-11
状态：已与用户确认

## 目标

用 Go 实现"Agent｜MCP+LLM+RAG｜智能体"的渐进式 demo 集合。
与既有项目 `D:\github\ai`（go-agent-from-scratch，纯手写无框架）互补，
本项目采用**官方/社区库为主**的实现风格，贴近生产实践。

## 已确认的关键决策

| 决策点 | 选择 |
|---|---|
| 项目名/位置 | `D:\github\example-go-agent-lab` |
| 实现风格 | 官方/社区库为主（mcp-go、openai-go、chromem-go） |
| LLM 接入 | OpenAI 兼容协议，DeepSeek/GLM/Qwen 经 BaseURL 切换 |
| 结构 | 单 go.mod + `cmd/` 四个 demo + `internal/` 共享代码 |
| 范围 | 四步递进：MCP Server → MCP Client+LLM → RAG → 综合智能体 |
| 向量方案 | chromem-go（嵌入式，免外部服务）+ OpenAI 兼容 /embeddings |
| Embedding | Qwen text-embedding-v4 / GLM embedding-3 / 硅基流动 bge-m3 可切换 |
| 语言 | 代码注释与 README 用中文 |

## 目录结构

```
example-go-agent-lab/
├── README.md                  # 总览 + 运行顺序引导
├── go.mod                     # module example-go-agent-lab, go 1.26
├── configs/config.yaml        # provider 配置（仿 ai 项目）
├── internal/
│   ├── llm/                   # OpenAI 兼容客户端封装（openai-go + WithBaseURL）
│   └── provider/              # deepseek/glm/qwen 三家注册表
├── cmd/
│   ├── 01-mcp-server/         # demo 1：MCP 服务器
│   ├── 02-mcp-agent/          # demo 2：MCP 客户端 + LLM 工具循环
│   ├── 03-rag/                # demo 3：检索增强问答
│   └── 04-combo-agent/        # demo 4：综合智能体
└── docs/                      # 每个 demo 一篇原理说明 + 本 spec
```

## Demo 设计

### 01-mcp-server — MCP 服务器

- 库：`github.com/mark3labs/mcp-go/server`
- 传输：stdio
- 工具（3 个，教学向）：
  - `get_time`：返回当前时间（可带时区参数）
  - `calculate`：四则运算表达式求值（安全的表达式解析，非 eval）
  - `read_note`：读取 `docs/notes/` 下指定文件名的笔记
- 验证：可被 MCP Inspector（`npx @modelcontextprotocol/inspector`）连接调试；
  也是 demo 02 的后端。

### 02-mcp-agent — MCP 客户端 + LLM 循环

- 库：`github.com/mark3labs/mcp-go/client`（stdio 子进程拉起 01 的 server）
- 流程：
  1. 启动子进程 → Initialize → ListTools
  2. MCP 工具 schema → OpenAI function calling tools 格式转换（纯函数，单测覆盖）
  3. Agent 循环：用户输入 → 模型返回 tool_calls → `CallTool` 执行 →
     结果以 tool role 回填 → 循环直到模型给出最终自然语言回答
- 交互：终端 REPL，多轮会话

### 03-rag — 检索增强问答

- 库：`github.com/philippgille/chromem-go`
- 流程：
  1. 加载 `docs/notes/` 下 md/txt 语料
  2. 切分：固定字符窗口 + 重叠（手写 splitter，纯函数 + 单测）
  3. Embedding（OpenAI 兼容 /embeddings）→ 入 chromem-go 集合
  4. 查询：query 向量化 → top-k 相似块 → 拼装上下文
  5. LLM 生成带来源引用的回答（system prompt 约束"仅基于给定资料"）
- 子命令：`ingest`（建库）/ `ask`（问答），避免每次查询重建库

### 04-combo-agent — 综合智能体

- 形态：demo 02 的 MCP Agent 循环 + demo 03 的 RAG
- 关键设计：**RAG 工具化** —— 将知识库检索包装为 MCP 工具
  `search_knowledge_base`，与 01 的工具统一注册进工具列表，
  由模型自主决定何时检索
- 增加多轮对话记忆（截断到最近 N 轮）
- 命令行参数可开关各工具组

## Provider 配置

`configs/config.yaml`：

```yaml
provider: deepseek        # deepseek | glm | qwen
embedding_provider: qwen  # qwen | glm | siliconflow（RAG 用；deepseek 无 embedding 接口）
```

环境变量（key 不落盘）：

| 环境变量 | 用途 |
|---|---|
| `DEEPSEEK_API_KEY` | 聊天（deepseek） |
| `ZHIPU_API_KEY` | 聊天/Embedding（glm） |
| `DASHSCOPE_API_KEY` | 聊天/Embedding（qwen） |
| `SILICONFLOW_API_KEY` | Embedding（siliconflow） |

### 端点注册表

| provider | BaseURL | 默认聊天模型 | 默认 Embedding 模型 |
|---|---|---|---|
| deepseek | `https://api.deepseek.com` | `deepseek-chat` | — |
| glm | `https://open.bigmodel.cn/api/paas/v4` | `glm-4.6` | `embedding-3` |
| qwen | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` | `text-embedding-v4` |
| siliconflow | `https://api.siliconflow.cn/v1` | — | `BAAI/bge-m3` |

模型名均可在 config.yaml 覆盖（模型迭代快，配置化防止过时）。

## 依赖清单

| 库 | 用途 |
|---|---|
| `github.com/mark3labs/mcp-go` | MCP server + client |
| `github.com/openai/openai-go` | LLM/Embedding（OpenAI 兼容） |
| `github.com/philippgille/chromem-go` | 嵌入式向量库 |

不引入 langchaingo（过重，且本 demo 各环节已可由轻量库覆盖）。

## 错误处理

- API key 缺失：启动时明确报错并指明需要的环境变量名
- MCP 子进程启动失败/崩溃：报错退出并附 stderr 摘要
- 工具执行出错：错误信息作为工具结果回填给模型（让模型自行处理），
  而非直接崩溃 —— 这是 Agent 韧性的教学点
- 检索无结果：返回明确"未命中"语义，由模型告知用户

## 测试策略

- 纯逻辑 TDD：splitter 切分、MCP→OpenAI 工具格式转换、表达式求值、
  provider 注册表读取
- 不对真实 API 做单测（需要 key 与网络）；四个 demo 以"可运行"为验收
- README 提供每个 demo 的预期输出示例

## 非目标（YAGNI）

- 不做 Web UI / HTTP API，仅终端交互
- 不做流式输出（现有 ai 项目已覆盖该主题）
- 不做独立向量数据库（Qdrant/Milvus）接入
- 不做多 Agent 协作、ReAct 显式提示词等专题（现有 ai 项目已覆盖 ReAct）

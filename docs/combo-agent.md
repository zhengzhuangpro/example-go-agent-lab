# demo 04：综合智能体（MCP 工具 + RAG 工具化 + 记忆截断）

对应代码：`cmd/04-combo-agent/main.go`。它是前三个 demo 的合流：
MCP 工具循环（demo02）+ 向量检索（demo03），再补两个生产化细节：
工具组开关与多轮记忆截断。

## 是什么：RAG 工具化 vs 固定前置流程

demo03 是**固定流程**：不管问什么，一律先检索 top-3、把资料塞进 system
prompt 再生成。这有两个代价：闲聊/算术问题也要白付一次 embedding 与更长的
上下文；检索词只能用用户原话，没有"要不要检索、检索什么"的判断空间。

demo04 把检索**工具化**：知识库查询被包装成一个名为
`search_knowledge_base` 的工具，与 MCP 工具（get_time/calculate/read_note）
注册进同一张工具表。**检索时机由模型自主决策**——问"RAG 切分策略"它检索，
问"(12+8)*5"它调计算器，打招呼它什么都不调。RAG 从"流程"变成了"能力"。

## 代码怎么走的

1. **聊天端点**：config → `provider.Chat` → `appenv.Lookup` 取 key（先查聊天
   provider 的变量，如 DEEPSEEK_API_KEY）→ `llm.New`。
2. **工具组一（`-mcp`，默认开）**：与 demo02 完全同构——`NewStdioMCPClient`
   拉起 demo01 子进程，Initialize 握手，tools/list 发现工具，
   `mcp2llm.ConvertTools` 转格式，handler 转发 `CallTool`。
   子进程被收进 `closers`，退出时统一 `Close`。
3. **工具组二（`-rag`，默认开）**：取 embedding 端点 key，`llm.NewEmbedder`，
   经 `chromem.EmbeddingFunc` 适配器打开 `.chromem/` 库的 "notes" 集合
   （与 demo03 **同一个库**，ingest 一次两个 demo 共用）；
   然后手工定义 `search_knowledge_base` 的 ToolDef 并注册 handler：
   取 `query` 参数 → `n := min(topK, col.Count())` 钳制后 `Query`
   （chromem v0.7.0 要求 nResults 不超过文档数；空库直接返回"未命中"提示）→
   结果拼成 `[编号] (来源, 相似度) 内容` 的文本回填。
4. **护栏**：两组都关掉时 `log.Fatal("至少启用一个工具组（-mcp 或 -rag）")`。
5. **Agent + REPL**：与 demo02 相同的 `agent.Agent`（MaxSteps=8、OnToolCall
   打印 `⚙` 轨迹），差异在每轮结束后 `history = agent.TruncateHistory(updated, *keepHistory)`。

## 关键教学点

- **schema 与 description 就是给模型的"使用说明书"**：
  `search_knowledge_base` 的 description 写明知识库**装了什么**
  （Go 并发/MCP/RAG/Agent 模式中文笔记）、**何时该调**（问题涉及这些主题）、
  **何时不调**（闲聊或常识），参数 `query` 带自己的 description。
  模型对"要不要检索"的判断几乎完全由这段文字驱动——工具化之后，
  写好 description 等价于把 demo03 里写死的流程控制升级为自然语言策略。
- **工具组开关**：`-mcp=false` 只留检索工具、`-rag=false` 只留 MCP 工具，
  是理解"工具表是运行时装配出来的"最直接的方式——Agent 本身不感知工具来源。
- **记忆截断与 tool 消息保护**：demo02 的 REPL 历史**刻意不设上限**（教学基线），
  demo04 每轮用 `TruncateHistory(updated, 12)` 只保留最近 12 条。截断有个协议
  陷阱：若切完的开头恰是 tool 消息，它配对的 assistant(tool_calls) 已被截掉，
  消息流非法、API 会拒绝——`TruncateHistory` 会继续丢弃开头的 tool 消息
  直到合法。`-history` 可调档，兼顾长记忆与 token 成本。
- **复用关系**：demo04 没有新写任何循环或向量库代码——agent 循环复用
  `internal/agent`（与 demo02 同一个 `agent.Agent`），向量库复用 demo03 的
  `.chromem/` 目录与集合、同一个 `EmbeddingFunc` 适配器，MCP 接线复用
  demo02 的客户端模式。组合层的代码量因此很小，这正是前三层抽象的回报。

## 怎么跑 / 怎么验证

```bash
export DEEPSEEK_API_KEY=sk-... DASHSCOPE_API_KEY=sk-...
go run ./cmd/03-rag ingest    # 先建库（demo04 复用同一 .chromem）
go run ./cmd/04-combo-agent
```

建议的验证序列：

```text
你> RAG 的切分策略有哪些讲究？   # 应触发 ⚙ search_knowledge_base(...)
你> 帮我算一下 (12+8)*5         # 应触发 ⚙ calculate(...)，得 100
你> 现在几点了？                 # 应触发 ⚙ get_time(...)
```

再跑 `go run ./cmd/04-combo-agent -rag=false`：启动日志只注册 MCP 工具组，
问 RAG 问题不会出现检索调用；`-history 4` 可观察长对话后记忆被截断。
多轮追问同一主题，模型仍能接上上文（最近消息还在窗口内）。

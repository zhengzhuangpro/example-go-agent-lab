# demo 02：MCP Agent（客户端 + LLM 工具循环）

对应代码：`cmd/02-mcp-agent/main.go`（依赖 `internal/agent`、`internal/mcp2llm`、
`internal/llm`、`internal/provider`、`internal/appenv`）。

## 是什么

demo02 把 demo01 与 LLM 连起来：作为 MCP **客户端**拉起 demo01 子进程，
把它的工具列表转换成 OpenAI function calling 格式交给模型，
进入 REPL 循环——用户提问，模型自主决定调用哪个工具，工具结果回填后再生成回答。
这一层"模型 ↔ 工具"的循环，就是 Agent 的最小形态。

## 代码怎么走的

1. **stdio 子进程模型**：`client.NewStdioMCPClient("go", os.Environ(),
   "run", "./cmd/01-mcp-server")` 创建客户端（尚未启动），`mcpc.Start(ctx)`
   真正拉起子进程，此后 JSON-RPC 走子进程的 stdin/stdout。
   `client.GetStderr` 可拿到子进程 stderr，`fatalWithStderr` 在启动/握手失败时
   打印它，便于定位 `go run` 编译错误之类的崩溃。
2. **协议握手**：`Initialize`（携带 `mcp.LATEST_PROTOCOL_VERSION` 与客户端信息）
   → 对方返回能力与服务器信息 → `ListTools` 拿到工具清单。这三步是 MCP 的
   标准连接时序，顺序不能乱。
3. **schema 转换**：`mcp2llm.ConvertTools(mcpTools)` 把 MCP 工具定义转成
   function calling 的 `tools` 数组——`InputSchema` 的 `type`/`properties`/`required`
   三个字段**原样平移**到 `function.parameters`（properties 为空时补空对象，
   description 有则带上）。MCP 的 JSON Schema 与 OpenAI 的参数 schema 本质同构，
   所以转换是纯函数，可直接单测。
4. **执行包装**：为每个工具注册一个 handler，转发 `mcpc.CallTool`，
   从结果 content 中提取 `mcp.TextContent` 拼成字符串。
5. **Agent 循环**（`internal/agent`）：`ag.Run(ctx, history, input)` 内部——
   system 消息注入头部 → 调 `Complete` → 若返回 `tool_calls`：
   构造带 tool_calls 的 assistant 消息写入两侧消息流 → 逐个执行工具、
   以 `role:"tool"`（带 call ID）回填结果 → 再次调 `Complete`；
   直到模型不再要工具，最终文本即回答。`MaxSteps=8` 防止无限循环。
6. **REPL**：`history = updated` 无截断地滚雪球（刻意保持简单，
   是教学基线；demo04 会演示截断版，见 `docs/combo-agent.md`）。

## 关键教学点

- **工具错误是数据，不是异常**：问 `1/0` 时，calculate 返回工具级错误
  `ERROR: 表达式无法计算: 除数为零`，agent 把它作为 tool 消息回填给模型，
  模型向用户如实解释——循环不中断、进程不崩溃。
  未知工具、参数 JSON 非法、handler 返回 error，都统一走这条"ERROR: 前缀回填"路径。
- **Complete 与 Handler 均为可注入接口**：`agent.CompleteFn` 是一个函数类型
  （`llm.Client.Complete` 天然满足签名），工具执行是 `map[string]agent.ToolHandler`。
  单元测试（`agent_test.go`）用假 Complete / 假 Handler 跑完整循环，
  不发任何网络请求，也不需要 MCP 子进程——这是"逻辑与副作用分离"的直接收益。
- **call ID 必须配对**：tool 消息必须带对应 tool_call 的 ID，否则协议非法、
  会被 API 拒绝。`llm.AssistantWithToolCalls` / `llm.ToolResult` 两个 helper
  把这层容易写错的构造封掉。

## 怎么跑 / 怎么验证

```bash
export DEEPSEEK_API_KEY=sk-...   # 或改 config.yaml 用 glm/qwen
go run ./cmd/02-mcp-agent
```

启动日志应显示发现的 3 个 MCP 工具。建议依次试：

```text
你> 现在几点了？北京时间        # 触发 get_time
你> 帮我算 (12+8)*5             # 触发 calculate，得 100
你> calculate 1/0 会怎样         # 工具错误作为数据回填，模型如实说明
```

每轮都会打印 `⚙ 工具名(参数)` 与 `← 结果` 轨迹；退出输入 `exit`。
不需要 key 的替代验证：直接按 `docs/mcp-server.md` 的管道命令测 demo01。

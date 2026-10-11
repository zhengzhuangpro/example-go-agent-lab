# demo 01：MCP 服务器（stdio）

对应代码：`cmd/01-mcp-server/main.go`（依赖 `internal/expr`）。

## 是什么

MCP（Model Context Protocol）是一个把"工具/资源"标准化暴露给模型的协议。
demo01 用社区库 [mcp-go](https://github.com/mark3labs/mcp-go) 的 server 包实现了一个
stdio 传输的 MCP 服务器，暴露三个教学工具：

| 工具 | 功能 | 教学点 |
|------|------|--------|
| `get_time` | 查当前时间（可选时区） | 参数枚举约束 + Windows 时区数据 |
| `calculate` | 四则运算表达式求值 | 手写词法 + 递归下降（绝不 eval） |
| `read_note` | 读 `docs/notes/` 下笔记 | 路径穿越防护 |

stdio 传输的工作方式：服务器从 **stdin 逐行读 JSON-RPC 请求**，
把响应写到 **stdout**；日志类信息走 stderr。
因此它可以被任何宿主（MCP 客户端、调试器）当普通子进程拉起，
不需要开端口，天然适合本地工具。

## 代码怎么走的

三个调用即成服务器（`main.go`）：

1. `server.NewMCPServer("notes-tools", "1.0.0", ...)` —— 创建服务器，
   `WithToolCapabilities(false)` 声明工具能力，`WithRecovery()` 让单个 handler
   panic 不至于打死整个进程。
2. `s.AddTool(tool, handler)` —— 注册工具与实现。工具 schema 用
   `mcp.NewTool` + `mcp.WithString`/`mcp.Description`/`mcp.Required` 等选项式 API 声明。
3. `server.ServeStdio(s)` —— 阻塞在 stdin 上，按行解析 JSON-RPC 并分发。

每个 handler 签名是 `func(ctx, mcp.CallToolRequest) (*mcp.CallToolResult, error)`，
用 `req.GetString("timezone", "")` / `req.RequireString("name")` 取参数，
用 `mcp.NewToolResultText` / `mcp.NewToolResultErrorf` 构造结果。

## 关键教学点

- **`get_time` 的参数约束**：`mcp.Enum("Asia/Shanghai", "UTC", ...)` 把合法取值
  写进 JSON Schema，宿主/模型侧就能拦截非法值；运行时再用 `time.LoadLocation`
  兜底校验，不认识仍返回工具级错误。`import _ "time/tzdata"` 是 Windows 必需的：
  Windows 没有系统级 IANA 时区库，内嵌 tzdata 后 `LoadLocation` 才不会失败。
- **`calculate` 为何不用 eval**：把模型生成的字符串直接当代码执行是注入漏洞。
  这里手写词法分析 + 递归下降解析（`internal/expr`），支持 `+ - * /`、括号、
  一元负号与小数；非法输入只会得到明确报错，不可能执行任意代码。
- **`read_note` 的路径穿越防护**：文件名来自模型，绝不能直接拼接路径。
  先 `filepath.Base(filepath.Clean(name))` 只保留文件名部分（`../secrets` 之类
  的目录成分被丢弃），再白名单校验扩展名（仅 `.md`/`.txt`），最后才读文件。
- **工具级错误 vs 传输级错误**：handler 返回 `NewToolResultError*` 时，
  结果的 `IsError` 被置位，JSON-RPC 层面仍是成功响应——错误文本作为**数据**
  发回给调用方（模型可读并自我纠正）；而返回非 nil `error` 是传输/协议级失败，
  对应 JSON-RPC error，适用于"基础设施坏了"而非"输入不合法"。demo01 的业务
  错误（除数为零、文件不存在）全部走前者。

## 怎么跑 / 怎么验证

不需要任何 API key，纯本地运行。手工喂 JSON-RPC（管道三条消息：
initialize 握手 → initialized 通知 → tools/call 调用）：

```bash
printf '%s\n%s\n%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
 '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
 '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"(1+2)*3"}}}' \
 | go run ./cmd/01-mcp-server
```

stdout 应答 initialize 与 tools/call 两条响应（后者 content 为 `9`）。
也可以用官方调试器交互式查看三个工具的 schema 并试调用：

```bash
npx @modelcontextprotocol/inspector go run ./cmd/01-mcp-server
```

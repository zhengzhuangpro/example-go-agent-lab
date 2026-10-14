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

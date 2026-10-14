# demo 04：综合智能体——把一切装进工具循环

## 这一步演示什么

前三步的合体：`runTurn` 工具循环（demo02）驱动两种来源的工具——

- **MCP 工具**（demo01）：子进程里的 get_time / calculate，经
  initialize → tools/list 发现 → tools/call 调用
- **本地工具**（demo03）：search_notes 直连 chromem 向量库

模型自主决定调哪个、调几次；Agent = LLM + 工具 + 循环。

## 核心概念

1. **schema 转换**：MCP 的 InputSchema 与 OpenAI function calling 的
   parameters 都是 JSON Schema，`mcpToolsToOpenAI` 做纯平移。
2. **工具即能力，来源无关**：循环只认 `func(ctx, map[string)any) string`，
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

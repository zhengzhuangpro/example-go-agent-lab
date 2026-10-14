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

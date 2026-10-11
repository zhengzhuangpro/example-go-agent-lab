# Agent 常见模式笔记

## Agent 循环

Agent = LLM + 工具 + 循环：用户输入 → 模型思考 → 返回 tool_calls →
执行工具 → 结果以 tool 角色回填 → 模型继续思考 → …… 直到模型给出
自然语言最终回答。每一轮循环叫一步（step）。

## function calling 协议要点

- 请求：tools 数组声明 name/description/parameters（JSON Schema）
- 响应：finish_reason=tool_calls，携带 tool_calls[].id/function.name/arguments
- 回填：assistant 消息带 tool_calls + 每个调用对应一条 tool 消息
  （tool_call_id 必须配对），模型据此生成下一条消息

## 工具错误是数据不是异常

工具执行失败时，把错误文本（如 "ERROR: 除数为零"）作为工具结果
回填给模型，让模型自己决定重试、换参数还是向用户解释——这比直接
崩溃更符合 Agent 的韧性要求。

## 记忆管理

多轮对话历史无限增长会撑爆上下文：常用策略是截断到最近 N 轮。
注意截断后开头不能是 tool 消息（它引用的 assistant 已被截掉，
协议上非法），需要继续丢弃直到开头不是 tool 消息。

## 步数上限

循环必须有 MaxSteps 保护（如 8 步），防止模型陷入无限调工具。

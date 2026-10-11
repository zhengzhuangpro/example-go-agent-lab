# MCP（Model Context Protocol）协议笔记

## 是什么

MCP 是 2024 年 11 月由 Anthropic 发布的开放协议，为 LLM 应用与外部
工具/数据源之间定义标准连接方式：服务器暴露能力，客户端（通常是
Agent/IDE）发现并调用。常被类比成"AI 应用的 USB-C 接口"。

## 传输层

- stdio：客户端把服务器作为子进程拉起，stdin/stdout 交换消息，本地最快
- Streamable HTTP：远程部署，HTTP POST + 可选 SSE 流

## 三类原语

1. tools：可被模型调用的函数（function calling 的服务端）
2. resources：只读数据源（文件、数据库行等）
3. prompts：可复用的提示词模板

## 协议细节

- 消息格式：JSON-RPC 2.0，请求/响应/通知三种
- 握手：客户端先发 initialize（带 protocolVersion 与 clientInfo），
  服务器回应能力声明，客户端再发 notifications/initialized 完成握手
- 工具发现：tools/list；工具调用：tools/call（name + arguments）

## 与 function calling 的关系

MCP 客户端把 tools/list 拿到的 JSON Schema 转换成 OpenAI function
calling 格式喂给模型；模型的 tool_calls 再经 tools/call 送回服务器。
MCP 负责"工具在哪里、怎么调"，function calling 负责"模型怎么决定调"。

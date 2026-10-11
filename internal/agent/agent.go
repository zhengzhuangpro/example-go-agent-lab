// Package agent 实现与传输无关的 LLM 工具循环：
// 模型返回 tool_calls → 执行工具 → 结果回填 → 再问模型，直到最终回答。
// Complete 与 Handler 均为可注入接口，单元测试用假实现，不碰真实 API。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/llm"
)

// ToolHandler 是一个工具的执行函数；错误会被当作数据回填给模型。
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// CompleteFn 抽象"把对话交给大模型"这一步（llm.Client.Complete 满足此签名）。
type CompleteFn func(ctx context.Context, msgs []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error)

// DefaultMaxSteps 防止模型无限调工具。
const DefaultMaxSteps = 8

// Agent 一次 Run 处理一轮用户输入。
type Agent struct {
	SystemPrompt string
	ToolDefs     []llm.ToolDef          // 给模型看的工具定义（name/schema）
	Handlers     map[string]ToolHandler // 工具名 → 执行函数
	Complete     CompleteFn
	MaxSteps     int
	OnToolCall   func(name, arguments, result string) // 工具调用轨迹回调（打印日志用）
}

// Run 处理一轮输入并返回（最终回答, 更新后的历史, 错误）。
// history 不含 system——SystemPrompt 每轮由 Agent 注入头部；
// 更新后的历史包含本轮 user、中间的 assistant/tool 消息与最终 assistant 回答。
func (a *Agent) Run(ctx context.Context, history []llm.Message, user string) (string, []llm.Message, error) {
	maxSteps := a.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	updated := make([]llm.Message, 0, len(history)+4)
	updated = append(updated, history...)
	updated = append(updated, openai.UserMessage(user))

	msgs := make([]llm.Message, 0, len(updated)+1)
	if a.SystemPrompt != "" {
		msgs = append(msgs, openai.SystemMessage(a.SystemPrompt))
	}
	msgs = append(msgs, updated...)

	for step := 0; step < maxSteps; step++ {
		content, calls, err := a.Complete(ctx, msgs, a.ToolDefs)
		if err != nil {
			return "", updated, err
		}
		if len(calls) == 0 {
			updated = append(updated, openai.AssistantMessage(content))
			return content, updated, nil
		}
		// 模型决定调用工具：assistant(tool_calls) 写入两侧消息流
		assistantMsg := llm.AssistantWithToolCalls(content, calls)
		msgs = append(msgs, assistantMsg)
		updated = append(updated, assistantMsg)
		for _, call := range calls {
			result := a.callTool(ctx, call)
			if a.OnToolCall != nil {
				a.OnToolCall(call.Name, call.Arguments, result)
			}
			toolMsg := llm.ToolResult(result, call.ID)
			msgs = append(msgs, toolMsg)
			updated = append(updated, toolMsg)
		}
	}
	return "", updated, fmt.Errorf("已达最大步数 %d 仍未得到最终回答", maxSteps)
}

// callTool 执行单个工具调用。
// 工具层错误（未知工具/参数非法/执行失败）不中断循环，统一以 "ERROR: ..." 回填。
func (a *Agent) callTool(ctx context.Context, call llm.ToolCall) string {
	handler, ok := a.Handlers[call.Name]
	if !ok {
		return fmt.Sprintf("ERROR: 未知工具 %q", call.Name)
	}
	var args map[string]any
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return fmt.Sprintf("ERROR: 工具参数不是合法 JSON: %v", err)
		}
	}
	result, err := handler(ctx, args)
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	return result
}

// TruncateHistory 保留最近 keepLast 条消息，用于控制多轮对话上下文长度。
// 若截断后开头是 tool 消息（其配对的 assistant 已被截掉，协议上非法），
// 会继续丢弃，直到开头不是 tool 消息。
func TruncateHistory(msgs []llm.Message, keepLast int) []llm.Message {
	if len(msgs) <= keepLast {
		return msgs
	}
	out := append([]llm.Message(nil), msgs[len(msgs)-keepLast:]...)
	for len(out) > 0 && out[0].OfTool != nil {
		out = out[1:]
	}
	return out
}

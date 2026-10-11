// Package mcp2llm 把 MCP tools/list 返回的工具定义
// 转换成 OpenAI function calling 的 tools 格式（纯函数，可单测）。
package mcp2llm

import (
	"github.com/mark3labs/mcp-go/mcp"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"example-go-agent-lab/internal/llm"
)

// ConvertTools 批量转换。MCP 的 InputSchema（JSON Schema 对象）
// 直接平移为 function.parameters 的 type/properties/required 字段。
func ConvertTools(tools []mcp.Tool) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		params := shared.FunctionParameters{
			"type":       t.InputSchema.Type,
			"properties": t.InputSchema.Properties,
		}
		if t.InputSchema.Properties == nil {
			params["properties"] = map[string]any{}
		}
		if len(t.InputSchema.Required) > 0 {
			params["required"] = t.InputSchema.Required
		}
		fd := shared.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: params,
		}
		if t.Description != "" {
			fd.Description = openai.String(t.Description)
		}
		out = append(out, openai.ChatCompletionFunctionTool(fd))
	}
	return out
}

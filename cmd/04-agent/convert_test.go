package main

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPToolsToOpenAI(t *testing.T) {
	tools := []mcp.Tool{{
		Name:        "get_time",
		Description: "获取当前时间",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"timezone": map[string]any{"type": "string"},
			},
			Required: []string{"timezone"},
		},
	}}
	got := mcpToolsToOpenAI(tools)
	if len(got) != 1 {
		t.Fatalf("got %d 个工具, want 1", len(got))
	}
	f := got[0].GetFunction()
	if f == nil {
		t.Fatal("应为 function 类型工具")
	}
	if f.Name != "get_time" {
		t.Errorf("name = %q", f.Name)
	}
	// JSON Schema 字段经序列化断言（这才是发给 API 的最终形状）
	b, err := json.Marshal(f.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(b, &params); err != nil {
		t.Fatal(err)
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v", params["type"])
	}
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["timezone"]; !ok {
		t.Errorf("properties 缺 timezone: %v", params["properties"])
	}
	req, _ := params["required"].([]any)
	if len(req) != 1 || req[0] != "timezone" {
		t.Errorf("required = %v", params["required"])
	}
}

func TestMCPToolsToOpenAIEmptyRequired(t *testing.T) {
	tools := []mcp.Tool{{
		Name:        "ping",
		Description: "",
		InputSchema: mcp.ToolInputSchema{Type: "object", Properties: map[string]any{}},
	}}
	got := mcpToolsToOpenAI(tools)
	b, _ := json.Marshal(got[0].GetFunction().Parameters)
	var params map[string]any
	_ = json.Unmarshal(b, &params)
	if _, exists := params["required"]; exists {
		t.Errorf("无必填参数时不应输出 required 字段: %s", b)
	}
}

package llm

import (
	"encoding/json"
	"testing"
)

// 经 JSON 断言而非直接读 union 字段：序列化形状才是发给 API 的最终事实。
func TestAssistantWithToolCalls(t *testing.T) {
	m := AssistantWithToolCalls("", []ToolCall{
		{ID: "call_1", Name: "calculate", Arguments: `{"expression":"1+2"}`},
	})
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["role"] != "assistant" {
		t.Errorf("role = %v, want assistant", got["role"])
	}
	calls, ok := got["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %v", got["tool_calls"])
	}
	c0 := calls[0].(map[string]any)
	if c0["id"] != "call_1" || c0["type"] != "function" {
		t.Errorf("tool_call = %v", c0)
	}
	fn := c0["function"].(map[string]any)
	if fn["name"] != "calculate" {
		t.Errorf("function.name = %v", fn["name"])
	}
	if fn["arguments"] != `{"expression":"1+2"}` {
		t.Errorf("function.arguments = %v", fn["arguments"])
	}
}

func TestToolResult(t *testing.T) {
	m := ToolResult("42", "call_9")
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["role"] != "tool" {
		t.Errorf("role = %v, want tool", got["role"])
	}
	if got["tool_call_id"] != "call_9" {
		t.Errorf("tool_call_id = %v", got["tool_call_id"])
	}
	if got["content"] != "42" {
		t.Errorf("content = %v", got["content"])
	}
}

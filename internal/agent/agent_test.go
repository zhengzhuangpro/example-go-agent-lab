package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"

	"example-go-agent-lab/internal/llm"
)

// scriptedComplete 按脚本依次返回模型响应，并捕获每次收到的消息列表。
type scriptedComplete struct {
	responses []struct {
		content string
		calls   []llm.ToolCall
	}
	captured [][]llm.Message
}

func (s *scriptedComplete) fn(ctx context.Context, msgs []llm.Message, tools []llm.ToolDef) (string, []llm.ToolCall, error) {
	cp := append([]llm.Message(nil), msgs...)
	s.captured = append(s.captured, cp)
	r := s.responses[len(s.captured)-1]
	return r.content, r.calls, nil
}

func TestRunDirectAnswer(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{{content: "你好！", calls: nil}}}
	ag := &Agent{Complete: sc.fn}
	answer, updated, err := ag.Run(context.Background(), nil, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "你好！" {
		t.Errorf("answer = %q", answer)
	}
	// 更新后的历史：user + assistant，各 1 条；system 不进历史
	if len(updated) != 2 {
		t.Fatalf("len(updated) = %d, want 2", len(updated))
	}
	// 未设置 SystemPrompt 时，发给模型的首条不应是 system（设置了才会注入前缀）
	first := sc.captured[0][0]
	if first.OfSystem != nil {
		t.Errorf("无 SystemPrompt 时首条不应是 system")
	}
}

func TestRunToolRoundTrip(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "call_1", Name: "calculate", Arguments: `{"expression":"1+2"}`}}},
		{content: "1+2=3"},
	}}
	var gotArgs map[string]any
	ag := &Agent{
		SystemPrompt: "测试",
		Handlers: map[string]ToolHandler{
			"calculate": func(ctx context.Context, args map[string]any) (string, error) {
				gotArgs = args
				return "3", nil
			},
		},
		Complete: sc.fn,
	}
	answer, updated, err := ag.Run(context.Background(), nil, "1+2等于几")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "1+2=3" {
		t.Fatalf("answer = %q", answer)
	}
	if gotArgs["expression"] != "1+2" {
		t.Errorf("handler 收到 args = %v", gotArgs)
	}
	// 历史：user、assistant(tool_calls)、tool、assistant(final)
	if len(updated) != 4 {
		t.Fatalf("len(updated) = %d, want 4", len(updated))
	}
	// 第二次调模型的入参里含 tool 结果消息（经 JSON 断言 union 内容）
	second := sc.captured[1]
	b, _ := json.Marshal(second)
	s := string(b)
	if !strings.Contains(s, `"role":"tool"`) || !strings.Contains(s, `"3"`) {
		t.Errorf("第二轮消息缺少 tool 结果: %s", s)
	}
	if !strings.Contains(s, `"tool_call_id":"call_1"`) {
		t.Errorf("tool 消息未带 tool_call_id: %s", s)
	}
}

func TestRunToolErrorIsData(t *testing.T) {
	// 工具执行报错不应中断循环，而是以 ERROR: 文本回填给模型
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "c1", Name: "bad", Arguments: `{}`}}},
		{content: "工具坏了"},
	}}
	ag := &Agent{
		Handlers: map[string]ToolHandler{
			"bad": func(ctx context.Context, args map[string]any) (string, error) {
				return "", context.DeadlineExceeded
			},
		},
		Complete: sc.fn,
	}
	answer, _, err := ag.Run(context.Background(), nil, "q")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "工具坏了" {
		t.Fatalf("answer = %q", answer)
	}
	b, _ := json.Marshal(sc.captured[1])
	if !strings.Contains(string(b), "ERROR:") {
		t.Errorf("工具错误未以 ERROR: 回填: %s", b)
	}
}

func TestRunUnknownTool(t *testing.T) {
	sc := &scriptedComplete{responses: []struct {
		content string
		calls   []llm.ToolCall
	}{
		{calls: []llm.ToolCall{{ID: "c1", Name: "nope", Arguments: `{}`}}},
		{content: "没这个工具"},
	}}
	ag := &Agent{Complete: sc.fn}
	if _, _, err := ag.Run(context.Background(), nil, "q"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(sc.captured[1])
	if !strings.Contains(string(b), "未知工具") {
		t.Errorf("未知工具未正确回填: %s", b)
	}
}

func TestRunMaxStepsExceeded(t *testing.T) {
	calls := []llm.ToolCall{{ID: "c1", Name: "loop", Arguments: `{}`}}
	sc := &scriptedComplete{}
	for i := 0; i < 20; i++ {
		sc.responses = append(sc.responses, struct {
			content string
			calls   []llm.ToolCall
		}{calls: calls})
	}
	ag := &Agent{
		Handlers: map[string]ToolHandler{
			"loop": func(ctx context.Context, args map[string]any) (string, error) { return "x", nil },
		},
		Complete: sc.fn,
	}
	if _, _, err := ag.Run(context.Background(), nil, "q"); err == nil {
		t.Fatal("超过最大步数应报错")
	} else if !strings.Contains(err.Error(), "最大步数") {
		t.Fatalf("错误应提及最大步数: %v", err)
	}
}

func TestTruncateHistory(t *testing.T) {
	u := openai.UserMessage("u")
	tcMsg := llm.AssistantWithToolCalls("", []llm.ToolCall{{ID: "1", Name: "n", Arguments: `{}`}})
	toolMsg := llm.ToolResult("r", "1")
	aMsg := openai.AssistantMessage("a")
	msgs := []llm.Message{u, tcMsg, toolMsg, aMsg, toolMsg, u}
	// keepLast=4 截到 [toolMsg, aMsg, toolMsg, u]，开头 tool 被继续丢弃，得 [aMsg, toolMsg, u]
	got := TruncateHistory(msgs, 4)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3（开头 tool 消息被继续丢弃）", len(got))
	}
	if got[0].OfAssistant == nil {
		t.Errorf("截断后首条应为 assistant 消息")
	}
	// 不超长时原样返回
	if got2 := TruncateHistory(msgs, 100); len(got2) != len(msgs) {
		t.Errorf("不超长不应截断")
	}
}

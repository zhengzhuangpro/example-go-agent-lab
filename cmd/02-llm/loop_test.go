package main

import (
	"context"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
)

// fakeComplete 用脚本回放模型行为：第 1 次要工具，第 2 次给最终回答。
func TestRunTurnToolLoop(t *testing.T) {
	var gotName string
	handlers := map[string]func(context.Context, map[string]any) string{
		"echo": func(ctx context.Context, args map[string]any) string {
			gotName, _ = args["name"].(string)
			return "hi"
		},
	}
	calls := 0
	complete := func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		calls++
		if calls == 1 {
			return "", []toolCall{{ID: "c1", Name: "echo", Arguments: `{"name":"世界"}`}}, nil
		}
		return "完成", nil, nil
	}
	answer, updated, err := runTurn(context.Background(), complete, nil, handlers, nil, "打招呼")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "完成" {
		t.Fatalf("answer = %q", answer)
	}
	if gotName != "世界" {
		t.Errorf("工具收到的参数 name = %q", gotName)
	}
	if calls != 2 {
		t.Errorf("complete 调用 %d 次, want 2", calls)
	}
	// 更新后的历史应含：user、assistant(tool_calls)、tool、assistant(final) 共 4 条
	if len(updated) != 4 {
		t.Errorf("updated 应有 4 条消息, got %d", len(updated))
	}
}

// 模型永远要工具时必须在步数上限处停下，不能死循环。
func TestRunTurnMaxSteps(t *testing.T) {
	complete := func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		return "", []toolCall{{ID: "c", Name: "echo", Arguments: "{}"}}, nil
	}
	handlers := map[string]func(context.Context, map[string]any) string{
		"echo": func(ctx context.Context, args map[string]any) string { return "x" },
	}
	_, _, err := runTurn(context.Background(), complete, nil, handlers, nil, "hi")
	if err == nil || !strings.Contains(err.Error(), "8") {
		t.Fatalf("应在 8 步上限处报错, got %v", err)
	}
}

func TestDispatch(t *testing.T) {
	handlers := map[string]func(context.Context, map[string]any) string{
		"ok": func(ctx context.Context, args map[string]any) string { return "fine" },
	}
	ctx := context.Background()
	if got := dispatch(ctx, handlers, toolCall{Name: "ok", Arguments: `{}`}); got != "fine" {
		t.Errorf("正常分发 = %q", got)
	}
	if got := dispatch(ctx, handlers, toolCall{Name: "nope", Arguments: `{}`}); !strings.HasPrefix(got, "ERROR:") {
		t.Errorf("未知工具应返回 ERROR: 前缀, got %q", got)
	}
	if got := dispatch(ctx, handlers, toolCall{Name: "ok", Arguments: `{bad`}); !strings.HasPrefix(got, "ERROR:") {
		t.Errorf("坏 JSON 应返回 ERROR: 前缀, got %q", got)
	}
	// 空参数字符串：视为无参数，不报 JSON 错
	if got := dispatch(ctx, handlers, toolCall{Name: "ok", Arguments: ""}); got != "fine" {
		t.Errorf("空 arguments 应按空参数处理, got %q", got)
	}
}

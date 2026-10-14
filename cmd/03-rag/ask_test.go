package main

import (
	"testing"

	openai "github.com/openai/openai-go/v3"
)

// 空 Choices 必须报错而非 panic——与 02/04 的 completeFnOf 同一防御标准。
func TestAnswerTextEmptyChoices(t *testing.T) {
	if _, err := answerText(&openai.ChatCompletion{}); err == nil {
		t.Fatal("空 Choices 应返回错误, 而非索引越界 panic")
	}
	resp := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "答"}}},
	}
	got, err := answerText(resp)
	if err != nil {
		t.Fatal(err)
	}
	if got != "答" {
		t.Fatalf("got %q, want %q", got, "答")
	}
}

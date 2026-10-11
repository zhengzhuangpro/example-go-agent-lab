// Package llm 封装 OpenAI 兼容的聊天与 embedding 接口。
// 通过 option.WithBaseURL 切换 DeepSeek/GLM/Qwen/硅基流动等兼容服务。
package llm

import (
	"context"
	"fmt"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Message 是发给聊天接口的一条消息（openai-go 参数 union 的别名）。
type Message = openai.ChatCompletionMessageParamUnion

// ToolDef 是传给模型的工具定义（function calling 格式）。
type ToolDef = openai.ChatCompletionToolUnionParam

// ToolCall 是模型返回的一次工具调用请求。
type ToolCall struct {
	ID        string // 工具调用 ID，回填结果时必须带上
	Name      string
	Arguments string // JSON 字符串，由调用方解析
}

// AssistantWithToolCalls 构造"带 tool_calls 的 assistant 消息"，
// 用于把模型的工具调用决定写回对话历史。
func AssistantWithToolCalls(content string, calls []ToolCall) Message {
	tcParams := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, c := range calls {
		tcParams = append(tcParams, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: c.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      c.Name,
					Arguments: c.Arguments,
				},
			},
		})
	}
	return Message{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(content),
			},
			ToolCalls: tcParams,
		},
	}
}

// ToolResult 构造 tool 角色的结果消息，callID 必须与对应的 tool_call 一致。
func ToolResult(content, callID string) Message {
	return openai.ToolMessage(content, callID)
}

// Client 是聊天客户端。
type Client struct {
	sc    openai.Client
	model string
}

// New 创建聊天客户端；baseURL 形如 https://api.deepseek.com。
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		sc: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
		),
		model: model,
	}
}

// Complete 发送对话并返回回答文本与工具调用。
// FinishReason 为 tool_calls 时 calls 非空；模型给出最终回答时 calls 为空。
func (c *Client) Complete(ctx context.Context, msgs []Message, tools []ToolDef) (string, []ToolCall, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: msgs,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	resp, err := c.sc.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", nil, fmt.Errorf("调用聊天接口失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("模型未返回任何候选回复")
	}
	choice := resp.Choices[0]
	var calls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" {
			continue
		}
		calls = append(calls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return choice.Message.Content, calls, nil
}

// Embedder 是向量化客户端。
type Embedder struct {
	sc    openai.Client
	model string
}

// NewEmbedder 创建 embedding 客户端（走 /embeddings 兼容接口）。
func NewEmbedder(baseURL, apiKey, model string) *Embedder {
	return &Embedder{
		sc: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
		),
		model: model,
	}
}

// Embed 批量向量化。chromem-go 需要 []float32，这里完成 float64→float32 转换。
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	resp, err := e.sc.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("调用 embedding 接口失败: %w", err)
	}
	out := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		v := make([]float32, len(d.Embedding))
		for j, f := range d.Embedding {
			v[j] = float32(f)
		}
		out[i] = v
	}
	return out, nil
}

// demo 04：综合智能体 = 02 的工具循环 + 01 的 MCP 工具 + 03 的 RAG 检索（工具化）。
//
// 三种能力装进同一个 runTurn 循环，模型自主决定调用哪个：
//   - get_time / calculate —— 来自 MCP 子进程（demo01 的 serve）
//   - search_notes ———— 本进程直连 chromem 向量库（demo03 建的库）
//
// 运行（先跑过 demo03 的 ingest）：
//
//	go run ./cmd/04-agent
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/config"
)

const topK = 3

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ctx := context.Background()

	// ── 聊天模型 ─────────────────────────────────────────────
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	complete := completeFnOf(sc, model)

	toolDefs := []openai.ChatCompletionToolUnionParam{}
	handlers := map[string]func(context.Context, map[string]any) string{}

	// ── 工具来源一：MCP 子进程（demo01）──────────────────────
	mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp", "serve")
	if err != nil {
		log.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpc.Close()
	if err := mcpc.Start(ctx); err != nil {
		log.Fatalf("启动 MCP 子进程失败: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "04-agent", Version: "1.0.0"}
	if _, err := mcpc.Initialize(ctx, initReq); err != nil {
		log.Fatalf("MCP initialize 失败: %v", err)
	}
	toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("tools/list 失败: %v", err)
	}
	toolDefs = append(toolDefs, mcpToolsToOpenAI(toolsRes.Tools)...)
	for _, t := range toolsRes.Tools {
		log.Printf("注册 MCP 工具: %s", t.Name)
		handlers[t.Name] = func(ctx context.Context, args map[string]any) string {
			res, err := mcpc.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: t.Name, Arguments: args},
			})
			if err != nil {
				return "ERROR: " + err.Error()
			}
			var sb strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					sb.WriteString(tc.Text)
				}
			}
			if res.IsError { // 工具级错误也是数据
				return "ERROR: " + sb.String()
			}
			return sb.String()
		}
	}

	// ── 工具来源二：RAG 检索（demo03 建的库，本进程直连）─────
	eep, ekey, emodel, err := cfg.Embedding()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)
	esc := openai.NewClient(option.WithAPIKey(ekey), option.WithBaseURL(eep.BaseURL))
	db, err := chromem.NewPersistentDB(".chromem", false)
	if err != nil {
		log.Fatalf("打开向量库失败: %v", err)
	}
	col, err := db.GetOrCreateCollection("notes", nil, embedFn(esc, emodel))
	if err != nil {
		log.Fatalf("创建集合失败: %v", err)
	}
	toolDefs = append(toolDefs, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name: "search_notes",
		Description: openai.String(
			"在项目知识库（MCP 协议、RAG 基础、Agent 模式、Go 并发等中文笔记）中检索资料。" +
				"用户问题涉及这些主题时调用；闲聊或常识问题不必调用。"),
		Parameters: shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"query": shared.FunctionParameters{"type": "string", "description": "检索关键词或问题"},
			},
			"required": []string{"query"},
		},
	}))
	handlers["search_notes"] = func(ctx context.Context, args map[string]any) string {
		q, _ := args["query"].(string)
		if strings.TrimSpace(q) == "" {
			return "ERROR: 缺少 query 参数"
		}
		results, err := retrieve(ctx, col, q) // 空库由 retrieve 钳制，安静返回空
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if len(results) == 0 {
			return "未命中任何资料（知识库可能尚未 ingest，可提示用户先运行 go run ./cmd/03-rag ingest）。"
		}
		var sb strings.Builder
		for i, r := range results {
			fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n", i+1, r.Metadata["source"], r.Similarity, r.Content)
		}
		return sb.String()
	}
	log.Printf("注册本地工具: search_notes")

	// ── REPL：三种工具，一个循环 ─────────────────────────────
	fmt.Println("综合智能体已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []openai.ChatCompletionMessageParamUnion
	for {
		fmt.Print("你> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		answer, updated, err := runTurn(ctx, complete, toolDefs, handlers, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

// mcpToolsToOpenAI：MCP 的 InputSchema（JSON Schema）平移为
// OpenAI function calling 的 tools 格式——纯函数，可单测。
func mcpToolsToOpenAI(tools []mcp.Tool) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
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
		fd := shared.FunctionDefinitionParam{Name: t.Name, Parameters: params}
		if t.Description != "" {
			fd.Description = openai.String(t.Description)
		}
		out = append(out, openai.ChatCompletionFunctionTool(fd))
	}
	return out
}

// ── 以下复制自 demo02 / demo03——教学项目宁复制，不抽象 ──────────

// completeFnOf 把 openai-go 的调用包成 completeFn：
// 返回回答文本与精简后的 tool_calls（finish_reason=tool_calls 时非空）。
func completeFnOf(sc openai.Client, model string) completeFn {
	return func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error) {
		params := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(model),
			Messages: msgs,
		}
		if len(tools) > 0 {
			params.Tools = tools
		}
		resp, err := sc.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", nil, err
		}
		if len(resp.Choices) == 0 {
			return "", nil, fmt.Errorf("模型未返回任何候选回复")
		}
		var calls []toolCall
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			if tc.Type == "function" {
				calls = append(calls, toolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
			}
		}
		return resp.Choices[0].Message.Content, calls, nil
	}
}

// toolCall 是模型返回的一次工具调用请求。
type toolCall struct {
	ID        string
	Name      string
	Arguments string // JSON 字符串
}

type completeFn func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error)

// runTurn 处理一轮用户输入：模型要工具就执行并回填，直到模型给出纯文本回答。
// history 不含 system（system 每次由调用方拼在最前）。
func runTurn(ctx context.Context, complete completeFn, tools []openai.ChatCompletionToolUnionParam, handlers map[string]func(context.Context, map[string]any) string, history []openai.ChatCompletionMessageParamUnion, user string) (string, []openai.ChatCompletionMessageParamUnion, error) {
	msgs := append(history, openai.UserMessage(user))
	for step := 0; step < 8; step++ {
		content, calls, err := complete(ctx, msgs, tools)
		if err != nil {
			return "", history, err
		}
		if len(calls) == 0 { // 模型不再要工具 = 最终回答
			msgs = append(msgs, openai.AssistantMessage(content))
			return content, msgs, nil
		}
		// 模型决定调工具：assistant(tool_calls) 与每条结果都要写回消息流
		msgs = append(msgs, assistantWithToolCalls(content, calls))
		for _, c := range calls {
			result := dispatch(ctx, handlers, c)
			fmt.Printf("  ⚙ %s(%s)\n  ← %s\n", c.Name, c.Arguments, firstLine(result))
			msgs = append(msgs, openai.ToolMessage(result, c.ID))
		}
	}
	return "", history, fmt.Errorf("已达最大步数 8，模型仍在要求调用工具")
}

// dispatch 执行单个工具调用；一切错误都变成 ERROR: 文本回填给模型。
func dispatch(ctx context.Context, handlers map[string]func(context.Context, map[string]any) string, c toolCall) string {
	h, ok := handlers[c.Name]
	if !ok {
		return fmt.Sprintf("ERROR: 未知工具 %q", c.Name)
	}
	var args map[string]any
	if strings.TrimSpace(c.Arguments) != "" {
		if err := json.Unmarshal([]byte(c.Arguments), &args); err != nil {
			return fmt.Sprintf("ERROR: 工具参数不是合法 JSON: %v", err)
		}
	}
	return h(ctx, args)
}

// assistantWithToolCalls 构造"带 tool_calls 的 assistant 消息"写回历史。
func assistantWithToolCalls(content string, calls []toolCall) openai.ChatCompletionMessageParamUnion {
	tc := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(calls))
	for _, c := range calls {
		tc = append(tc, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: c.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      c.Name,
					Arguments: c.Arguments,
				},
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(content),
			},
			ToolCalls: tc,
		},
	}
}

// firstLine 取结果首行（超 120 rune 截断），让 REPL 轨迹可读。
func firstLine(s string) string {
	line := strings.SplitN(s, "\n", 2)[0]
	if r := []rune(line); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return line
}

// embedFn 把 openai-go 的 /embeddings 调用适配成 chromem 要的 EmbeddingFunc。
func embedFn(sc openai.Client, model string) chromem.EmbeddingFunc {
	return chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
		resp, err := sc.Embeddings.New(ctx, openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: []string{text}},
			Model: openai.EmbeddingModel(model),
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Data) == 0 {
			return nil, fmt.Errorf("embedding 接口未返回向量")
		}
		v := make([]float32, len(resp.Data[0].Embedding))
		for i, f := range resp.Data[0].Embedding {
			v[i] = float32(f)
		}
		return v, nil
	})
}

// retrieve 检索 top-k 相关块。chromem 要求 1 <= n <= 库内文档数，
// 所以先钳制；空库安静返回空，由调用方给"未命中"提示。
func retrieve(ctx context.Context, col *chromem.Collection, query string) ([]chromem.Result, error) {
	n := min(topK, col.Count())
	if n == 0 {
		return nil, nil
	}
	return col.Query(ctx, query, n, nil, nil)
}

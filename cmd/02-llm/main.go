// demo 02：纯 LLM function calling——工具就是进程内的普通 Go 函数。
//
// 对比 demo01：MCP 把工具放进另一个进程、用 JSON-RPC 调用；
// 这里没有协议层，工具注册为 Go func，由模型通过 tool_calls 驱动。
// runTurn 的"模型要工具 → 执行 → 结果回填 → 再问模型"循环是所有 Agent 的心脏。
//
// 运行：go run ./cmd/02-llm   （需要聊天 API key，见 README）
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
	"time"
	_ "time/tzdata"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"example-go-agent-lab/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)

	// openai-go 客户端：WithBaseURL 一行切换 DeepSeek/GLM/Qwen 等 OpenAI 兼容服务
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	complete := completeFnOf(sc, model)

	tools := []openai.ChatCompletionToolUnionParam{timeTool(), calcTool()}
	handlers := map[string]func(map[string]any) string{
		"get_time":  handleGetTime,
		"calculate": handleCalculate,
	}

	fmt.Println("LLM 工具循环已就绪（输入 exit 退出）")
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
		answer, updated, err := runTurn(context.Background(), complete, tools, handlers, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

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

// ── 工具循环（Agent 的心脏）───────────────────────────────────────

// toolCall 是模型返回的一次工具调用请求。
type toolCall struct {
	ID        string
	Name      string
	Arguments string // JSON 字符串
}

type completeFn func(ctx context.Context, msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam) (string, []toolCall, error)

// runTurn 处理一轮用户输入：模型要工具就执行并回填，直到模型给出纯文本回答。
// history 不含 system（system 每次由调用方拼在最前）。
func runTurn(ctx context.Context, complete completeFn, tools []openai.ChatCompletionToolUnionParam, handlers map[string]func(map[string]any) string, history []openai.ChatCompletionMessageParamUnion, user string) (string, []openai.ChatCompletionMessageParamUnion, error) {
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
			result := dispatch(handlers, c)
			fmt.Printf("  ⚙ %s(%s)\n  ← %s\n", c.Name, c.Arguments, firstLine(result))
			msgs = append(msgs, openai.ToolMessage(result, c.ID))
		}
	}
	return "", history, fmt.Errorf("已达最大步数 8，模型仍在要求调用工具")
}

// dispatch 执行单个工具调用；一切错误都变成 ERROR: 文本回填给模型。
func dispatch(handlers map[string]func(map[string]any) string, c toolCall) string {
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
	return h(args)
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

// ── 两个进程内工具 ────────────────────────────────────────────────

// fnTool 小助手：拼一个 function calling 工具定义。
func fnTool(name, desc string, props shared.FunctionParameters, required ...string) openai.ChatCompletionToolUnionParam {
	if len(required) > 0 {
		props["required"] = required
	}
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        name,
		Description: openai.String(desc),
		Parameters:  props,
	})
}

func timeTool() openai.ChatCompletionToolUnionParam {
	return fnTool("get_time",
		"获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。",
		shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"timezone": shared.FunctionParameters{"type": "string", "description": "IANA 时区名"},
			},
		})
}

func calcTool() openai.ChatCompletionToolUnionParam {
	return fnTool("calculate",
		"计算四则运算表达式的值，支持 + - * /、括号与一元负号。",
		shared.FunctionParameters{
			"type": "object",
			"properties": shared.FunctionParameters{
				"expression": shared.FunctionParameters{"type": "string", "description": "算术表达式，如 1+2*3"},
			},
		}, "expression")
}

// 工具的错误也返回字符串——错误是回填给模型的数据，不是异常。
// （timeIn 复制自 demo01——教学项目宁复制，不抽象）
func timeIn(tz string) (string, error) {
	loc := time.Local
	if tz != "" {
		var err error
		if loc, err = time.LoadLocation(tz); err != nil {
			return "", fmt.Errorf("无法识别时区 %q", tz)
		}
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%s（%s，星期%s）",
		now.Format("2006-01-02 15:04:05"), now.Format("MST"),
		string([]rune("日一二三四五六")[int(now.Weekday())])), nil
}

func handleGetTime(args map[string]any) string {
	tz, _ := args["timezone"].(string)
	text, err := timeIn(tz)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return text
}

func handleCalculate(args map[string]any) string {
	exprStr, _ := args["expression"].(string)
	val, err := evaluate(exprStr)
	if err != nil {
		return "ERROR: 表达式无法计算: " + err.Error()
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
}

// firstLine 取结果首行（超 120 rune 截断），让 REPL 轨迹可读。
func firstLine(s string) string {
	line := strings.SplitN(s, "\n", 2)[0]
	if r := []rune(line); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return line
}

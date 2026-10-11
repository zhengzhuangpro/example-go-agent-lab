// demo 02：MCP 客户端 + LLM 工具循环。
// 拉起 demo01 作为 stdio MCP 子进程，把它的工具转成 function calling
// 交给模型，模型返回 tool_calls 时经 MCP CallTool 执行并回填。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"example-go-agent-lab/internal/agent"
	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/mcp2llm"
	"example-go-agent-lab/internal/provider"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ep, err := provider.Chat(cfg.Provider)
	if err != nil {
		log.Fatalf("聊天 provider 无效: %v", err)
	}
	apiKey, err := appenv.Lookup(ep.APIKeyEnv)
	if err != nil {
		log.Fatal(err)
	}
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = ep.DefaultChatModel
	}
	chat := llm.New(ep.BaseURL, apiKey, chatModel)
	log.Printf("聊天模型: %s（%s）", chatModel, ep.Name)

	ctx := context.Background()

	// 1. 拉起 demo01 MCP 服务器子进程（stdio 传输）
	mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp-server")
	if err != nil {
		log.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpc.Close()
	if err := mcpc.Start(ctx); err != nil {
		fatalWithStderr(mcpc, "启动 MCP 子进程失败: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "02-mcp-agent", Version: "1.0.0"}
	if _, err := mcpc.Initialize(ctx, initReq); err != nil {
		fatalWithStderr(mcpc, "MCP initialize 握手失败: %v", err)
	}

	// 2. 发现工具并转换成 function calling 格式
	toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fatalWithStderr(mcpc, "tools/list 失败: %v", err)
	}
	mcpTools := toolsRes.Tools
	names := make([]string, len(mcpTools))
	for i, t := range mcpTools {
		names[i] = t.Name
	}
	log.Printf("发现 %d 个 MCP 工具: %s", len(mcpTools), strings.Join(names, ", "))

	// 3. 每个工具的执行包装：转发到 MCP CallTool
	handlers := make(map[string]agent.ToolHandler)
	for _, t := range mcpTools {
		handlers[t.Name] = func(ctx context.Context, args map[string]any) (string, error) {
			res, err := mcpc.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: t.Name, Arguments: args},
			})
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					sb.WriteString(tc.Text)
				}
			}
			// 工具级错误（IsError）也作为 error 返回：agent.callTool 会统一加 "ERROR: " 前缀回填给模型
			if res.IsError {
				return "", fmt.Errorf("%s", sb.String())
			}
			return sb.String(), nil
		}
	}

	// 4. Agent + REPL
	ag := &agent.Agent{
		SystemPrompt: "你是一个能调用 MCP 工具的中文助手。涉及当前时间、算术计算、项目笔记查询的问题优先使用工具；工具结果出错时向用户如实说明。回答保持简洁。",
		ToolDefs:     mcp2llm.ConvertTools(mcpTools),
		Handlers:     handlers,
		Complete:     chat.Complete,
		MaxSteps:     8,
		OnToolCall: func(name, args, result string) {
			fmt.Printf("  ⚙ %s(%s)\n", name, args)
			fmt.Printf("  ← %s\n", truncateLines(result, 3))
		},
	}

	fmt.Println("MCP Agent 已就绪（输入 exit 退出）")
	scanner := bufio.NewScanner(os.Stdin)
	var history []llm.Message
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
		answer, updated, err := ag.Run(ctx, history, input)
		if err != nil {
			fmt.Printf("⚠ 出错: %v\n", err)
			continue
		}
		history = updated
		fmt.Printf("助手> %s\n", answer)
	}
}

// fatalWithStderr 报错时附带 MCP 子进程的 stderr 摘要，便于定位崩溃原因。
func fatalWithStderr(c *client.Client, format string, args ...any) {
	if r, ok := client.GetStderr(c); ok {
		if tail, err := io.ReadAll(r); err == nil && len(tail) > 0 {
			log.Fatalf(format+"；子进程 stderr:\n%s", append(args, string(tail))...)
		}
	}
	log.Fatalf(format, args...)
}

// truncateLines 把多行文本压成最多 maxLines 行（每行再截 120 字符）。
func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	kept := lines[:maxLines]
	for i, l := range kept {
		r := []rune(l)
		if len(r) > 120 {
			kept[i] = string(r[:120]) + "…"
		}
	}
	return strings.Join(kept, "\n") + fmt.Sprintf("\n  …（共 %d 行，已截断）", len(lines))
}

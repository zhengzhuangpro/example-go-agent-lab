// demo 04：综合智能体 = MCP 工具循环 + RAG 工具化。
// 关键设计：知识库检索不是固定的前置流程，而是被包装为
// search_knowledge_base 工具，与 MCP 工具统一注册，由模型自主决定何时检索。
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
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/agent"
	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/mcp2llm"
	"example-go-agent-lab/internal/provider"
)

const topK = 3

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	useMCP := flag.Bool("mcp", true, "启用 MCP 工具组（demo01 的 get_time/calculate/read_note）")
	useRAG := flag.Bool("rag", true, "启用知识库检索工具 search_knowledge_base")
	keepHistory := flag.Int("history", 12, "多轮记忆保留的最近消息条数")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	ctx := context.Background()

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

	toolDefs := []llm.ToolDef{}
	handlers := map[string]agent.ToolHandler{}
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	// ── 工具组一：MCP（demo01 子进程）──────────────────────────
	if *useMCP {
		mcpc, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp-server")
		if err != nil {
			log.Fatalf("创建 MCP 客户端失败: %v", err)
		}
		closers = append(closers, mcpc)
		if err := mcpc.Start(ctx); err != nil {
			fatalWithStderr(mcpc, "启动 MCP 子进程失败: %v", err)
		}
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "04-combo-agent", Version: "1.0.0"}
		if _, err := mcpc.Initialize(ctx, initReq); err != nil {
			fatalWithStderr(mcpc, "MCP initialize 握手失败: %v", err)
		}
		toolsRes, err := mcpc.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			fatalWithStderr(mcpc, "tools/list 失败: %v", err)
		}
		for _, t := range toolsRes.Tools {
			log.Printf("注册 MCP 工具: %s", t.Name)
		}
		toolDefs = append(toolDefs, mcp2llm.ConvertTools(toolsRes.Tools)...)
		for _, t := range toolsRes.Tools {
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
				return sb.String(), nil
			}
		}
	}

	// ── 工具组二：RAG 工具化 ────────────────────────────────────
	if *useRAG {
		eep, err := provider.Embedding(cfg.EmbeddingProvider)
		if err != nil {
			log.Fatalf("embedding provider 无效: %v", err)
		}
		ekey, err := appenv.Lookup(eep.APIKeyEnv)
		if err != nil {
			log.Fatal(err)
		}
		emodel := cfg.EmbeddingModel
		if emodel == "" {
			emodel = eep.DefaultEmbeddingModel
		}
		embedder := llm.NewEmbedder(eep.BaseURL, ekey, emodel)
		log.Printf("embedding 模型: %s（%s）", emodel, eep.Name)

		ef := chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
			vs, err := embedder.Embed(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			return vs[0], nil
		})
		db, err := chromem.NewPersistentDB(".chromem", false)
		if err != nil {
			log.Fatalf("打开向量库失败: %v", err)
		}
		col, err := db.GetOrCreateCollection("notes", nil, ef)
		if err != nil {
			log.Fatalf("创建集合失败: %v", err)
		}

		toolDefs = append(toolDefs, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: "search_knowledge_base",
			Description: openai.String(
				"在项目知识库（Go 并发、MCP 协议、RAG 基础、Agent 模式等中文笔记）中检索相关资料。" +
					"当用户的问题可能涉及这些主题时调用；闲聊或常识问题不必调用。"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": shared.FunctionParameters{
					"query": shared.FunctionParameters{
						"type":        "string",
						"description": "检索关键词或问题",
					},
				},
				"required": []string{"query"},
			},
		}))
		handlers["search_knowledge_base"] = func(ctx context.Context, args map[string]any) (string, error) {
			q, _ := args["query"].(string)
			if strings.TrimSpace(q) == "" {
				return "", fmt.Errorf("缺少 query 参数")
			}
			// chromem v0.7.0 要求 1 <= nResults <= 集合内文档数：
			// 用 Count() 钳制 topK；空库直接返回"未命中"，不把库错误抛给模型。
			n := min(topK, col.Count())
			if n == 0 {
				return "未命中任何资料（知识库可能尚未 ingest，可提示用户先运行 go run ./cmd/03-rag ingest）。", nil
			}
			results, err := col.Query(ctx, q, n, nil, nil)
			if err != nil {
				return "", err
			}
			if len(results) == 0 {
				return "未命中任何资料（知识库可能尚未 ingest，可提示用户先运行 go run ./cmd/03-rag ingest）。", nil
			}
			var sb strings.Builder
			for i, r := range results {
				fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n",
					i+1, r.Metadata["source"], r.Similarity, r.Content)
			}
			return sb.String(), nil
		}
		log.Printf("注册 RAG 工具: search_knowledge_base")
	}

	if len(toolDefs) == 0 {
		log.Fatal("至少启用一个工具组（-mcp 或 -rag）")
	}

	ag := &agent.Agent{
		SystemPrompt: "你是一个综合智能体，可以：查时间、算算术、读笔记（MCP 工具），以及检索项目知识库（search_knowledge_base）。" +
			"自主判断何时调用哪个工具；涉及项目笔记主题（Go 并发/MCP/RAG/Agent 模式）时优先检索知识库；" +
			"工具结果出错时向用户如实说明。用中文简洁回答。",
		ToolDefs: toolDefs,
		Handlers: handlers,
		Complete: chat.Complete,
		MaxSteps: 8,
		OnToolCall: func(name, args, result string) {
			fmt.Printf("  ⚙ %s(%s)\n", name, args)
			fmt.Printf("  ← %s\n", firstLine(result, 120))
		},
	}

	fmt.Println("综合智能体已就绪（输入 exit 退出）")
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
		// 多轮记忆：截断到最近 N 条（demo02 不截断，这里是增量教学点）
		history = agent.TruncateHistory(updated, *keepHistory)
		fmt.Printf("助手> %s\n", answer)
	}
}

func fatalWithStderr(c *client.Client, format string, args ...any) {
	if r, ok := client.GetStderr(c); ok {
		if tail, err := io.ReadAll(r); err == nil && len(tail) > 0 {
			log.Fatalf(format+"；子进程 stderr:\n%s", append(args, string(tail))...)
		}
	}
	log.Fatalf(format, args...)
}

func firstLine(s string, maxRunes int) string {
	line := strings.SplitN(s, "\n", 2)[0]
	r := []rune(line)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return line
}

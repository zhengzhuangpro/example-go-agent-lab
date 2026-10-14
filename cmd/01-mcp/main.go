// demo 01：MCP 服务器 + 客户端（stdio 传输）。不需要任何 API key。
//
// MCP 的核心思想：把"工具"放进独立进程，双方在 stdin/stdout 上用 JSON-RPC 对话：
// initialize 握手 → tools/list 发现工具 → tools/call 调用工具。
//
// 用法：
//
//	go run ./cmd/01-mcp serve   # 启动服务器（读 stdin / 写 stdout）
//	go run ./cmd/01-mcp call    # 拉起 serve 子进程，完整走一遍客户端流程
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // Windows 没有系统时区库，内嵌 tzdata 保证 LoadLocation 可用

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve()
	case "call":
		call()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: go run ./cmd/01-mcp <serve|call>")
}

// ── serve：MCP 服务器 ──────────────────────────────────────────────

func serve() {
	s := server.NewMCPServer("demo-tools", "1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	registerGetTime(s)
	registerCalculate(s)
	// ServeStdio 阻塞运行：stdin 收 JSON-RPC 请求，stdout 写响应。
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, "服务器退出:", err)
		os.Exit(1)
	}
}

// get_time：当前时间，可选 IANA 时区。
func registerGetTime(s *server.MCPServer) {
	tool := mcp.NewTool("get_time",
		mcp.WithDescription("获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。"),
		mcp.WithString("timezone",
			mcp.Description("IANA 时区名"),
			mcp.Enum("Asia/Shanghai", "UTC", "America/New_York", "Europe/London", "Asia/Tokyo"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := timeIn(req.GetString("timezone", ""))
		if err != nil {
			// 工具级错误：IsError=true 的正常响应，错误文本交给对方，连接不断
			return mcp.NewToolResultErrorf("%v", err), nil
		}
		return mcp.NewToolResultText(text), nil
	})
}

// timeIn 纯函数（便于单测）：把"现在"格式化到指定时区。
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

// calculate：四则运算（求值器见 expr.go）。
func registerCalculate(s *server.MCPServer) {
	tool := mcp.NewTool("calculate",
		mcp.WithDescription("计算四则运算表达式的值，支持 + - * /、括号与一元负号，例如 (1+2)*3。"),
		mcp.WithString("expression",
			mcp.Required(),
			mcp.Description("算术表达式，如 1+2*3"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := req.RequireString("expression")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("参数错误", err), nil
		}
		val, err := evaluate(raw)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("表达式无法计算", err), nil
		}
		// 去尾零：9.000000 → 9，2.500000 → 2.5
		text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
		return mcp.NewToolResultText(text), nil
	})
}

// ── call：MCP 客户端演示 ──────────────────────────────────────────

func call() {
	ctx := context.Background()
	// 拉起自己的 serve 作为子进程：stdio 传输 = 子进程的 stdin/stdout 就是信道
	c, err := client.NewStdioMCPClient("go", os.Environ(), "run", "./cmd/01-mcp", "serve")
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		log.Fatalf("启动子进程失败: %v", err)
	}

	// 1. initialize 握手：交换协议版本与能力
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "01-mcp-call", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		log.Fatalf("initialize 失败: %v", err)
	}

	// 2. tools/list：发现服务器有哪些工具
	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("tools/list 失败: %v", err)
	}
	for _, t := range res.Tools {
		fmt.Printf("发现工具: %s — %s\n", t.Name, t.Description)
	}

	// 3. tools/call：正常调用、错误作为数据返回（连接不会断）
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"get_time", map[string]any{"timezone": "Asia/Shanghai"}},
		{"calculate", map[string]any{"expression": "(1+2)*3"}},
		{"calculate", map[string]any{"expression": "1/0"}}, // 错误即数据
	} {
		callRes, err := c.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: tc.name, Arguments: tc.args},
		})
		if err != nil {
			log.Fatalf("tools/call %s 失败: %v", tc.name, err)
		}
		var sb strings.Builder
		for _, content := range callRes.Content {
			if txt, ok := content.(mcp.TextContent); ok {
				sb.WriteString(txt.Text)
			}
		}
		mark := "结果"
		if callRes.IsError {
			mark = "工具级错误（注意：连接没断，错误只是数据）"
		}
		fmt.Printf("调用 %s(%v)\n  → %s: %s\n", tc.name, tc.args, mark, sb.String())
	}
}

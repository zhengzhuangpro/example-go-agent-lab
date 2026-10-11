// demo 01：MCP 服务器（stdio 传输）。
// 提供 get_time / calculate / read_note 三个教学工具。
// 验证方式：printf 管道喂 JSON-RPC（见 README），或用 MCP Inspector 连接。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // Windows 没有系统时区库，内嵌 tzdata 保证 LoadLocation 可用

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"example-go-agent-lab/internal/expr"
)

func main() {
	s := server.NewMCPServer(
		"notes-tools",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	registerGetTime(s)
	registerCalculate(s)
	registerReadNote(s)
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, "服务器退出:", err)
		os.Exit(1)
	}
}

// get_time：获取当前时间，可选 IANA 时区。
func registerGetTime(s *server.MCPServer) {
	tool := mcp.NewTool("get_time",
		mcp.WithDescription("获取当前时间。可传入 IANA 时区名（如 Asia/Shanghai），不传则用本地时区。"),
		mcp.WithString("timezone",
			mcp.Description("IANA 时区名"),
			mcp.Enum("Asia/Shanghai", "UTC", "America/New_York", "Europe/London", "Asia/Tokyo"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tz := req.GetString("timezone", "")
		loc := time.Local
		if tz != "" {
			var err error
			loc, err = time.LoadLocation(tz)
			if err != nil {
				return mcp.NewToolResultErrorf("无法识别时区 %q: %v", tz, err), nil
			}
		}
		now := time.Now().In(loc)
		text := fmt.Sprintf("%s（%s，星期%s）",
			now.Format("2006-01-02 15:04:05"), now.Format("MST"),
			string([]rune("日一二三四五六")[int(now.Weekday())]))
		return mcp.NewToolResultText(text), nil
	})
}

// calculate：四则运算表达式求值（安全的递归下降解析，非 eval）。
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
		val, err := expr.Evaluate(raw)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("表达式无法计算", err), nil
		}
		// 去掉多余的尾零：9.000000 → 9，2.500000 → 2.5
		text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", val), "0"), ".")
		return mcp.NewToolResultText(text), nil
	})
}

// read_note：读取 docs/notes/ 下的笔记全文。
func registerReadNote(s *server.MCPServer) {
	tool := mcp.NewTool("read_note",
		mcp.WithDescription("读取项目知识库笔记的完整内容。name 是 docs/notes/ 下的文件名，如 mcp-protocol.md。"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("笔记文件名（.md 或 .txt）"),
		),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("参数错误", err), nil
		}
		// 防路径穿越：只取文件名部分，丢弃任何目录成分
		safe := filepath.Base(filepath.Clean(name))
		switch strings.ToLower(filepath.Ext(safe)) {
		case ".md", ".txt":
		default:
			return mcp.NewToolResultErrorf("仅支持 .md/.txt 文件，got %q", safe), nil
		}
		data, err := os.ReadFile(filepath.Join("docs", "notes", safe))
		if err != nil {
			return mcp.NewToolResultErrorf("读取失败: %v", err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

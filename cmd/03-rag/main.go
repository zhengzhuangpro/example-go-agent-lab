// demo 03：RAG——检索增强问答。
//
//	ingest：docs/notes/*.md → 切块 → embedding → .chromem/（chromem-go 持久化）
//	ask：   问题 → 向量检索 top-k → 拼进 prompt → LLM 只依据资料作答并标注引用
//
// 运行：
//
//	go run ./cmd/03-rag ingest
//	go run ./cmd/03-rag ask "MCP 是什么"
//
// 注意：切换 embedding_provider/model 后旧向量库不兼容，需 `rm -rf .chromem` 重建。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/config"
)

const (
	chunkSize = 500 // 每块最大 rune 数
	overlap   = 80  // 相邻块重叠 rune 数：防止关键句正好被块边界切断
	topK      = 3   // 检索返回块数
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || (args[0] != "ingest" && args[0] != "ask") {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/03-rag <ingest|ask>")
		fmt.Fprintln(os.Stderr, "  ingest          建库（docs/notes → .chromem/）")
		fmt.Fprintln(os.Stderr, "  ask <问题...>   基于知识库问答")
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}

	// embedding 与聊天可以是两家：deepseek 没有 /embeddings
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

	ctx := context.Background()
	if args[0] == "ingest" {
		if err := ingest(ctx, col); err != nil {
			log.Fatalf("ingest 失败: %v", err)
		}
		return
	}

	question := strings.Join(args[1:], " ")
	if strings.TrimSpace(question) == "" {
		log.Fatal(`ask 需要跟一个问题，例如: go run ./cmd/03-rag ask "MCP 是什么"`)
	}
	ep, apiKey, model, err := cfg.Chat()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("聊天模型: %s（%s）", model, ep.Name)
	sc := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(ep.BaseURL))
	if err := ask(ctx, sc, model, col, question); err != nil {
		log.Fatalf("ask 失败: %v", err)
	}
}

// chunk：固定 rune 窗口 + 重叠切块。按 rune 计数保证中文不被从字节中间切断。
func chunk(text string, size, overlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	step := size - overlap
	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := min(start+size, len(runes))
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
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

// ingest 建库：ID 取 "文件名#序号"，重复 ingest 按 ID 覆盖（幂等）。
func ingest(ctx context.Context, col *chromem.Collection) error {
	files, _ := filepath.Glob("docs/notes/*.md")
	var docs []chromem.Document
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", f, err)
		}
		base := filepath.Base(f)
		chunks := chunk(string(data), chunkSize, overlap)
		for i, ch := range chunks {
			docs = append(docs, chromem.Document{
				ID:       fmt.Sprintf("%s#%04d", base, i),
				Content:  ch,
				Metadata: map[string]string{"source": base},
			})
		}
		log.Printf("已切分 %s: %d 块", base, len(chunks))
	}
	if len(docs) == 0 {
		return fmt.Errorf("docs/notes/ 下没有找到 .md 语料")
	}
	if err := col.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		return fmt.Errorf("写入向量库: %w", err)
	}
	log.Printf("入库完成：共 %d 块（已持久化到 .chromem/）", len(docs))
	return nil
}

// ask 检索 + 生成：system prompt 约束"只依据资料作答"，回答标注 [n] 引用。
func ask(ctx context.Context, sc openai.Client, model string, col *chromem.Collection, question string) error {
	results, err := retrieve(ctx, col, question)
	if err != nil {
		return fmt.Errorf("检索失败: %w", err)
	}
	if len(results) == 0 {
		fmt.Println("未命中任何资料。请先运行: go run ./cmd/03-rag ingest")
		return nil
	}

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n", i+1, r.Metadata["source"], r.Similarity, r.Content)
	}
	system := "你是知识库问答助手。只依据下面提供的资料回答问题；引用资料时标注编号（如 [1]）；" +
		"如果资料不足以回答，明确说明资料未覆盖，不要编造。\n\n资料：\n" + sb.String()

	resp, err := sc.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(question),
		},
	})
	if err != nil {
		return err
	}
	fmt.Printf("助手> %s\n\n命中块：\n", resp.Choices[0].Message.Content)
	for i, r := range results {
		fmt.Printf("  [%d] %s（相似度 %.3f）\n", i+1, r.Metadata["source"], r.Similarity)
	}
	return nil
}

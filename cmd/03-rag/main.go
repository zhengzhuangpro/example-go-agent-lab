// demo 03：检索增强问答（RAG）。
// ingest：加载 docs/notes 语料 → 切分 → embedding → chromem-go 持久化
// ask：问题向量化 → top-k 检索 → 拼上下文 → LLM 生成带引用的回答
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
	"github.com/philippgille/chromem-go"

	"example-go-agent-lab/internal/appenv"
	"example-go-agent-lab/internal/config"
	"example-go-agent-lab/internal/llm"
	"example-go-agent-lab/internal/provider"
	"example-go-agent-lab/internal/splitter"
)

const (
	chunkSize = 500 // 每块最大 rune 数
	overlap   = 80  // 相邻块重叠 rune 数
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

	// embedding 端点独立于聊天端点（deepseek 无 embedding 接口）
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

	// chromem-go 的 EmbeddingFunc 适配器：内部走我们的 OpenAI 兼容 Embedder
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

	ctx := context.Background()
	if args[0] == "ingest" {
		if err := ingest(ctx, col); err != nil {
			log.Fatalf("ingest 失败: %v", err)
		}
		return
	}
	question := strings.Join(args[1:], " ")
	if strings.TrimSpace(question) == "" {
		log.Fatal("ask 需要跟一个问题，例如: go run ./cmd/03-rag ask MCP 是什么")
	}
	if err := ask(ctx, col, cfg, question); err != nil {
		log.Fatalf("ask 失败: %v", err)
	}
}

// ingest 建库：ID 取 "文件名#序号"，重复 ingest 按相同 ID 覆盖更新（幂等）。
func ingest(ctx context.Context, col *chromem.Collection) error {
	var docs []chromem.Document
	for _, pat := range []string{"docs/notes/*.md", "docs/notes/*.txt"} {
		files, _ := filepath.Glob(pat)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return fmt.Errorf("读取 %s: %w", f, err)
			}
			chunks, err := splitter.Split(string(data), chunkSize, overlap)
			if err != nil {
				return fmt.Errorf("切分 %s: %w", f, err)
			}
			base := filepath.Base(f)
			for i, ch := range chunks {
				docs = append(docs, chromem.Document{
					ID:       fmt.Sprintf("%s#%04d", base, i),
					Content:  ch,
					Metadata: map[string]string{"source": base},
				})
			}
			log.Printf("已切分 %s: %d 块", base, len(chunks))
		}
	}
	if len(docs) == 0 {
		return fmt.Errorf("docs/notes/ 下没有找到 .md/.txt 语料")
	}
	if err := col.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		return fmt.Errorf("写入向量库: %w", err)
	}
	log.Printf("入库完成：共 %d 块（已持久化到 .chromem/）", len(docs))
	return nil
}

// ask 检索 + 生成。
func ask(ctx context.Context, col *chromem.Collection, cfg config.Config, question string) error {
	// chromem v0.7.0 要求 nResults 不超过集合内文档数，空库或小块数会直接报错；
	// 先用 Count() 钳制，空库时给出对用户友好的"未命中"提示。
	n := min(topK, col.Count())
	if n == 0 {
		fmt.Println("未命中任何资料。请先运行: go run ./cmd/03-rag ingest")
		return nil
	}
	results, err := col.Query(ctx, question, n, nil, nil)
	if err != nil {
		return fmt.Errorf("检索失败: %w", err)
	}
	if len(results) == 0 {
		fmt.Println("未命中任何资料。请先运行: go run ./cmd/03-rag ingest")
		return nil
	}

	var ctxSB strings.Builder
	for i, r := range results {
		fmt.Fprintf(&ctxSB, "[%d] (来源: %s, 相似度 %.3f)\n%s\n\n",
			i+1, r.Metadata["source"], r.Similarity, r.Content)
	}

	ep, err := provider.Chat(cfg.Provider)
	if err != nil {
		return err
	}
	key, err := appenv.Lookup(ep.APIKeyEnv)
	if err != nil {
		return err
	}
	chatModel := cfg.ChatModel
	if chatModel == "" {
		chatModel = ep.DefaultChatModel
	}
	chat := llm.New(ep.BaseURL, key, chatModel)

	system := "你是知识库问答助手。只依据下面提供的资料回答问题；引用资料时标注编号（如 [1]）；" +
		"如果资料不足以回答，明确说明资料未覆盖，不要编造。\n\n资料：\n" + ctxSB.String()
	answer, _, err := chat.Complete(ctx, []llm.Message{
		openai.SystemMessage(system),
		openai.UserMessage(question),
	}, nil)
	if err != nil {
		return err
	}

	fmt.Printf("助手> %s\n\n", answer)
	fmt.Println("命中块：")
	for i, r := range results {
		fmt.Printf("  [%d] %s（相似度 %.3f）\n", i+1, r.Metadata["source"], r.Similarity)
	}
	return nil
}

# demo 03：RAG——检索增强问答

## 这一步演示什么

LLM 不知道你的私有资料，且会一本正经地编造。RAG 的解法分两步：

- `ingest`：把 `docs/notes/*.md` 切块、向量化、存进 chromem-go（嵌入式向量库）
- `ask`：把问题向量化，检索最相似的 top-k 块，拼进 prompt，
  要求模型只依据资料作答并标注引用

## 核心概念

1. **切块（chunk）**：embedding 模型对超长文本的表征会稀释；
   固定 rune 窗口 + 80 rune 重叠，防止关键句被块边界切断。
2. **向量检索**：文档与问题都变成同一空间的向量，用余弦相似度找近邻；
   chromem-go 是嵌入式库——无外部服务，一个目录就是库。
3. **引用式回答**：system prompt 约束"只依据资料、标注 [n]、不足就说未覆盖"，
   把"生成"锚定在"检索"上，压幻觉。
4. **空库钳制**：chromem 要求 `1 <= n <= 库内文档数`，所以检索前先
   `min(topK, Count())`；空库给"请先 ingest"而不是报错。

## 读码路线

`main.go` 从上往下：`chunk`（切块）→ `embedFn`（向量化适配）→
`retrieve`（检索+钳制）→ `ingest`（建库）→ `ask`（检索+生成）。

## 运行

    go run ./cmd/03-rag ingest
    go run ./cmd/03-rag ask "MCP 是什么"

需要 embedding 与聊天两类 API key（见 README）。切换 embedding 模型后
旧库不兼容：`rm -rf .chromem && go run ./cmd/03-rag ingest`。

## 与下一步的关系

本步的检索是"固定前置流程"：一问必检索。demo 04 把检索包装成
`search_notes` 工具，由模型自主决定何时检索——RAG 从流程变成了能力。

# demo 03：RAG（检索增强问答）

对应代码：`cmd/03-rag/main.go`（依赖 `internal/splitter`、`internal/llm`、
chromem-go）。语料在 `docs/notes/*.md|*.txt`（Go 并发、MCP 协议、RAG 基础、
Agent 模式四篇中文笔记）。

## 是什么

RAG = Retrieval-Augmented Generation：先从知识库里**检索**相关片段，
再把片段塞进 prompt 让模型**生成**带引用的回答。
它解决两个问题：模型不知道的私有知识（本项目笔记）、以及模型编造（幻觉）。
demo03 是最朴素的开局式 RAG：每次提问固定先检索、再回答。

## 代码怎么走的（文字版流程图）

ingest（建库，一次性）：

docs/notes 语料 → 逐文件读取 → Split(500, 80) 切块
→ Embedder 批量向量化 → chromem 持久化到 `.chromem/`（集合 "notes"）

ask（问答，每次）：

问题 → 向量化 → 集合内 Query top-3（余弦最近邻）
→ 命中块拼成带编号的"资料" → 注入 system prompt → 单次 Complete → 引用式回答

关键实现细节：

- **切分**：`splitter.Split(text, 500, 80)`——每块至多 500 个 **rune**（不是字节，
  中文不会被从中间切断），相邻块重叠 80 rune，防止关键句正好落在块边界。
- **embedding 适配**：chromem-go 只认 `func(ctx, string) ([]float32, error)`，
  而我们的 `llm.Embedder.Embed` 返回 `[][]float32`（OpenAI 兼容接口返回
  float64，已在 Embed 内转成 float32）。用 `chromem.EmbeddingFunc` 包一层
  适配器（取 `vs[0]`）即可接入，chromem 在写入与查询两侧都会调它。
- **幂等 ID**：每块 ID 取 `文件名#序号`（如 `mcp-protocol.md#0003`），
  metadata 记 `source=文件名`。重复 ingest 时相同 ID 覆盖更新，
  语料改了直接重跑 ingest 即可，不会越积越多。
- **检索前钳制**：chromem v0.7.0 要求 `1 <= nResults <= 集合文档数`，
  所以先 `n := min(topK, col.Count())`；`n == 0`（空库/未 ingest）时
  不报错，直接给用户"未命中任何资料"的提示。

## 关键教学点

- **相似度是什么**：chromem 用**余弦相似度**（cosine similarity）做最近邻，
  `chromem.Result.Similarity` 取值约在 0~1，越大越相关。它是语义空间的
  "方向接近度"，不是关键词重合——同义改写也能命中。
- **空检索的"未命中"语义**：检索不到时正确的做法是明说"资料未覆盖"，
  而不是让模型硬答。demo03 把"未命中"作为正常结果处理（提示先运行 ingest），
  与"检索接口报错"（异常路径）区分开。
- **"仅基于资料"的 system prompt 是抑制幻觉的关键**：
  `只依据下面提供的资料回答；引用时标注编号；资料不足时明确说明，不要编造`。
  没有这句约束，模型会用自己的参数知识"补全"答案且看起来同样自信；
  有了它，模型的职责从"知道答案"变成"整理证据"，越界空间被大幅压缩。
  当然 prompt 只是软约束，真正的硬保证还需要引用校验等工程手段。

## 怎么跑 / 怎么验证

```bash
export DASHSCOPE_API_KEY=sk-...   # embedding 用 qwen（config.yaml 可换 glm/siliconflow）
export DEEPSEEK_API_KEY=sk-...    # 聊天模型
go run ./cmd/03-rag ingest                     # 建库，日志显示每个文件的切块数
go run ./cmd/03-rag ask "RAG 的切分策略有哪些讲究？"
go run ./cmd/03-rag ask "今天股票行情怎么样"    # 语料外问题，应答"资料未覆盖"
```

验证要点：回答带 `[1]` 样式编号且能对上"命中块"列表；
语料外问题不编造；`.chromem/` 目录在 ingest 后出现（已 gitignore）。

注意：不同 embedding 模型的向量空间互不兼容——切换 embedding_provider/embedding_model 后，必须删除 `.chromem/` 并重新 `ingest`，否则检索结果是静默的垃圾。

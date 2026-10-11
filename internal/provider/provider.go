// Package provider 维护"OpenAI 兼容"端点注册表：
// 三家聊天（deepseek/glm/qwen）+ 三家 embedding（qwen/glm/siliconflow）。
// 模型迭代快，默认模型名集中在这里，config.yaml 可覆盖。
package provider

import "fmt"

// Endpoint 描述一个 OpenAI 兼容服务端点。
type Endpoint struct {
	Name                  string // 注册名，如 deepseek
	BaseURL               string // OpenAI 兼容 API 根地址
	APIKeyEnv             string // 存放 API key 的环境变量名
	DefaultChatModel      string // 默认聊天模型（空 = 不提供聊天）
	DefaultEmbeddingModel string // 默认 embedding 模型（空 = 不提供 embedding）
}

var chatEndpoints = map[string]Endpoint{
	"deepseek": {Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKeyEnv: "DEEPSEEK_API_KEY", DefaultChatModel: "deepseek-chat"},
	"glm":      {Name: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIKeyEnv: "ZHIPU_API_KEY", DefaultChatModel: "glm-4.6", DefaultEmbeddingModel: "embedding-3"},
	"qwen":     {Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKeyEnv: "DASHSCOPE_API_KEY", DefaultChatModel: "qwen-plus", DefaultEmbeddingModel: "text-embedding-v4"},
}

var embeddingEndpoints = map[string]Endpoint{
	"qwen":        chatEndpoints["qwen"],
	"glm":         chatEndpoints["glm"],
	"siliconflow": {Name: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1", APIKeyEnv: "SILICONFLOW_API_KEY", DefaultEmbeddingModel: "BAAI/bge-m3"},
}

// Chat 返回指定聊天 provider 的端点信息。
func Chat(name string) (Endpoint, error) {
	ep, ok := chatEndpoints[name]
	if !ok {
		return Endpoint{}, fmt.Errorf("未知聊天 provider %q（可选: deepseek | glm | qwen）", name)
	}
	return ep, nil
}

// Embedding 返回指定 embedding provider 的端点信息。
// deepseek 不提供 /embeddings 接口，聊天用 deepseek 时 embedding 需另选一家。
func Embedding(name string) (Endpoint, error) {
	ep, ok := embeddingEndpoints[name]
	if !ok {
		return Endpoint{}, fmt.Errorf("未知 embedding provider %q（可选: qwen | glm | siliconflow；deepseek 无 embedding 接口）", name)
	}
	return ep, nil
}

// Package config 是本项目唯一的共享包：
// OpenAI 兼容服务的端点表 + 配置加载（config.yaml + 本地覆盖 config.local.yaml）。
// 四个 demo 里凡是要连 LLM/embedding 的，都从这里拿端点、key 和模型名。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Endpoint 描述一个 OpenAI 兼容服务端点。
// DefaultEmbeddingModel 为空表示该端点不提供 /embeddings（如 deepseek）。
type Endpoint struct {
	Name                  string
	BaseURL               string
	APIKeyEnv             string
	DefaultChatModel      string
	DefaultEmbeddingModel string
}

// endpoints 聊天与 embedding 共用一张表。
var endpoints = map[string]Endpoint{
	"deepseek":    {Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKeyEnv: "DEEPSEEK_API_KEY", DefaultChatModel: "deepseek-chat"},
	"glm":         {Name: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIKeyEnv: "ZHIPU_API_KEY", DefaultChatModel: "glm-4.6", DefaultEmbeddingModel: "embedding-3"},
	"qwen":        {Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKeyEnv: "DASHSCOPE_API_KEY", DefaultChatModel: "qwen-plus", DefaultEmbeddingModel: "text-embedding-v4"},
	"siliconflow": {Name: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1", APIKeyEnv: "SILICONFLOW_API_KEY", DefaultEmbeddingModel: "BAAI/bge-m3"},
}

// Config 是 yaml 配置的形状。ChatModel/EmbeddingModel 空则用端点默认值。
type Config struct {
	Provider          string            `yaml:"provider"`           // 聊天: deepseek | glm | qwen
	EmbeddingProvider string            `yaml:"embedding_provider"` // 嵌入: qwen | glm | siliconflow
	ChatModel         string            `yaml:"chat_model"`         // 可选：覆盖聊天模型名
	EmbeddingModel    string            `yaml:"embedding_model"`    // 可选：覆盖 embedding 模型名
	APIKeys           map[string]string `yaml:"api_keys"`           // 本地覆盖文件专用：环境变量名 → key
}

// Load 读 yaml；若同目录存在 <名>.local.yaml（如 config.yaml → config.local.yaml），
// 再叠加其中的 api_keys。本地文件应加入 .gitignore，真实 key 绝不进版本库。
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	localPath := localPathFor(path)
	if localData, err := os.ReadFile(localPath); err == nil {
		var local Config
		if err := yaml.Unmarshal(localData, &local); err != nil {
			return cfg, fmt.Errorf("解析本地覆盖文件 %s 失败: %w", localPath, err)
		}
		for k, v := range local.APIKeys {
			if cfg.APIKeys == nil {
				cfg.APIKeys = map[string]string{}
			}
			cfg.APIKeys[k] = v
		}
	}

	if cfg.Provider == "" {
		cfg.Provider = "deepseek"
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = "qwen"
	}
	return cfg, nil
}

// localPathFor 把 config.yaml 映射为同目录的 config.local.yaml。
func localPathFor(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".local" + ext
}

// Chat 解析聊天端点：返回端点、API key（环境变量优先）、模型名（可覆盖）。
func (c Config) Chat() (Endpoint, string, string, error) {
	ep, ok := endpoints[c.Provider]
	if !ok || ep.DefaultChatModel == "" {
		return Endpoint{}, "", "", fmt.Errorf("未知聊天 provider %q（可选: deepseek | glm | qwen）", c.Provider)
	}
	key, err := c.resolveKey(ep.APIKeyEnv)
	if err != nil {
		return Endpoint{}, "", "", err
	}
	model := c.ChatModel
	if model == "" {
		model = ep.DefaultChatModel
	}
	return ep, key, model, nil
}

// Embedding 解析 embedding 端点（deepseek 无此能力，需另选一家）。
func (c Config) Embedding() (Endpoint, string, string, error) {
	ep, ok := endpoints[c.EmbeddingProvider]
	if !ok || ep.DefaultEmbeddingModel == "" {
		return Endpoint{}, "", "", fmt.Errorf("未知 embedding provider %q（可选: qwen | glm | siliconflow；deepseek 无 embedding 接口）", c.EmbeddingProvider)
	}
	key, err := c.resolveKey(ep.APIKeyEnv)
	if err != nil {
		return Endpoint{}, "", "", err
	}
	model := c.EmbeddingModel
	if model == "" {
		model = ep.DefaultEmbeddingModel
	}
	return ep, key, model, nil
}

// resolveKey 按 env > 本地文件 的顺序解析一个 API key。
func (c Config) resolveKey(envName string) (string, error) {
	if v := os.Getenv(envName); v != "" {
		return v, nil
	}
	if v := c.APIKeys[envName]; v != "" {
		return v, nil
	}
	return "", fmt.Errorf("缺少 %s 的 API key：请设置环境变量 %s，或在 configs/config.local.yaml（已 gitignore）中配置 api_keys.%s", envName, envName, envName)
}

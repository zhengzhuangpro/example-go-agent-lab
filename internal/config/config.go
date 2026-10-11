// Package config 读取 configs/config.yaml。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 是全项目共享的运行配置。
// ChatModel/EmbeddingModel 为空时使用 provider 注册表里的默认模型（可覆盖，防模型名过时）。
type Config struct {
	Provider          string `yaml:"provider"`           // deepseek | glm | qwen
	EmbeddingProvider string `yaml:"embedding_provider"` // qwen | glm | siliconflow
	ChatModel         string `yaml:"chat_model"`         // 可选：覆盖聊天模型名
	EmbeddingModel    string `yaml:"embedding_model"`    // 可选：覆盖 embedding 模型名
}

// Load 从 yaml 文件读取配置并填充默认值。
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	if cfg.Provider == "" {
		cfg.Provider = "deepseek"
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = "qwen"
	}
	return cfg, nil
}

// Package config 读取 configs/config.yaml，并叠加本地覆盖文件 configs/config.local.yaml。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 是全项目共享的运行配置。
// ChatModel/EmbeddingModel 为空时使用 provider 注册表里的默认模型（可覆盖，防模型名过时）。
// APIKeys 来自 gitignore 的本地覆盖文件，键为环境变量名（如 DEEPSEEK_API_KEY）。
type Config struct {
	Provider          string            `yaml:"provider"`           // deepseek | glm | qwen
	EmbeddingProvider string            `yaml:"embedding_provider"` // qwen | glm | siliconflow
	ChatModel         string            `yaml:"chat_model"`         // 可选：覆盖聊天模型名
	EmbeddingModel    string            `yaml:"embedding_model"`    // 可选：覆盖 embedding 模型名
	APIKeys           map[string]string `yaml:"api_keys"`           // 可选：本地文件的 API key（env 变量优先）
}

// Load 从 yaml 文件读取配置并填充默认值；
// 若同目录存在 <名>.local.yaml（如 config.yaml → config.local.yaml），
// 再叠加其中的 api_keys（该文件应加入 .gitignore，避免 key 落入版本库）。
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
		if len(local.APIKeys) > 0 {
			if cfg.APIKeys == nil {
				cfg.APIKeys = map[string]string{}
			}
			for k, v := range local.APIKeys {
				cfg.APIKeys[k] = v
			}
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

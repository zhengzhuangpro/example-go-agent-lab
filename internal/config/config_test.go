package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadExplicit(t *testing.T) {
	path := writeTemp(t, "provider: glm\nembedding_provider: siliconflow\nchat_model: glm-4-flash\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "glm" || cfg.EmbeddingProvider != "siliconflow" || cfg.ChatModel != "glm-4-flash" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	path := writeTemp(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "deepseek" {
		t.Errorf("默认 provider 应为 deepseek，got %q", cfg.Provider)
	}
	if cfg.EmbeddingProvider != "qwen" {
		t.Errorf("默认 embedding_provider 应为 qwen，got %q", cfg.EmbeddingProvider)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("no/such/file.yaml"); err == nil {
		t.Fatal("文件不存在应报错")
	}
}

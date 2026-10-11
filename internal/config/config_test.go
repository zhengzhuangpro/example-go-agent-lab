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

func TestLoadLocalOverlay(t *testing.T) {
	// config.local.yaml 只承载 api_keys：存在时覆盖合并，主文件其余字段不受影响
	dir := t.TempDir()
	main := filepath.Join(dir, "config.yaml")
	local := filepath.Join(dir, "config.local.yaml")
	os.WriteFile(main, []byte("provider: glm\n"), 0o644)
	os.WriteFile(local, []byte("api_keys:\n  DEEPSEEK_API_KEY: sk-from-local\n"), 0o644)
	cfg, err := Load(main)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "glm" {
		t.Errorf("主文件字段应保留, got provider=%q", cfg.Provider)
	}
	if cfg.APIKeys["DEEPSEEK_API_KEY"] != "sk-from-local" {
		t.Errorf("api_keys 应来自 local 覆盖文件, got %+v", cfg.APIKeys)
	}
}

func TestLoadLocalOverlayAbsent(t *testing.T) {
	// local 文件不存在是正常状态：api_keys 为空，不报错
	path := writeTemp(t, "provider: deepseek\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKeys != nil {
		t.Errorf("无 local 文件时 api_keys 应为 nil, got %+v", cfg.APIKeys)
	}
}

func TestLoadLocalOverlayBadYaml(t *testing.T) {
	// local 文件存在但解析失败：明确报错（静默忽略会让人以为 key 生效了）
	dir := t.TempDir()
	main := filepath.Join(dir, "config.yaml")
	local := filepath.Join(dir, "config.local.yaml")
	os.WriteFile(main, []byte("provider: deepseek\n"), 0o644)
	os.WriteFile(local, []byte("api_keys: [broken\n"), 0o644)
	if _, err := Load(main); err == nil {
		t.Fatal("local 文件格式错误应报错")
	}
}

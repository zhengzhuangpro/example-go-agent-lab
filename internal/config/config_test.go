package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCfg 在临时目录写 config.yaml 与可选的 config.local.yaml，返回主文件路径。
func writeCfg(t *testing.T, main, local string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if local != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.local.yaml"), []byte(local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "config.yaml")
}

func TestLoadLocalOverride(t *testing.T) {
	path := writeCfg(t,
		"provider: glm\nembedding_provider: qwen\n",
		"api_keys:\n  ZHIPU_API_KEY: sk-local\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "glm" {
		t.Errorf("provider = %q", cfg.Provider)
	}
	if cfg.APIKeys["ZHIPU_API_KEY"] != "sk-local" {
		t.Errorf("本地覆盖未生效: %v", cfg.APIKeys)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeCfg(t, "provider: glm\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingProvider != "qwen" {
		t.Errorf("embedding_provider 默认应为 qwen, got %q", cfg.EmbeddingProvider)
	}
}

func TestChatEnvPriority(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "sk-env")
	cfg := Config{Provider: "glm"}
	ep, key, model, err := cfg.Chat()
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-env" {
		t.Errorf("环境变量应优先于本地文件, key = %q", key)
	}
	if ep.BaseURL != "https://open.bigmodel.cn/api/paas/v4" || model != "glm-4.6" {
		t.Errorf("端点/模型解析错误: %+v %q", ep, model)
	}
}

func TestChatMissingKey(t *testing.T) {
	os.Unsetenv("ZHIPU_API_KEY")
	cfg := Config{Provider: "glm", APIKeys: map[string]string{"DEEPSEEK_API_KEY": "x"}}
	if _, _, _, err := cfg.Chat(); err == nil {
		t.Error("缺少 key 应报错")
	}
}

func TestChatUnknownProvider(t *testing.T) {
	cfg := Config{Provider: "nope"}
	if _, _, _, err := cfg.Chat(); err == nil {
		t.Error("未知 provider 应报错")
	}
}

func TestEmbeddingNoEmbeddingModel(t *testing.T) {
	cfg := Config{EmbeddingProvider: "deepseek"}
	if _, _, _, err := cfg.Embedding(); err == nil {
		t.Error("deepseek 无 embedding 模型应报错")
	}
}

func TestChatModelOverride(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "k")
	cfg := Config{Provider: "glm", ChatModel: "glm-5"}
	_, _, model, err := cfg.Chat()
	if err != nil {
		t.Fatal(err)
	}
	if model != "glm-5" {
		t.Errorf("model = %q, want glm-5", model)
	}
}

package provider

import "testing"

func TestChatKnownProviders(t *testing.T) {
	cases := []struct{ name, baseURL, keyEnv, model string }{
		{"deepseek", "https://api.deepseek.com", "DEEPSEEK_API_KEY", "deepseek-chat"},
		{"glm", "https://open.bigmodel.cn/api/paas/v4", "ZHIPU_API_KEY", "glm-4.6"},
		{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", "DASHSCOPE_API_KEY", "qwen-plus"},
	}
	for _, c := range cases {
		ep, err := Chat(c.name)
		if err != nil {
			t.Fatalf("Chat(%q) 不应报错: %v", c.name, err)
		}
		if ep.BaseURL != c.baseURL || ep.APIKeyEnv != c.keyEnv || ep.DefaultChatModel != c.model {
			t.Errorf("Chat(%q) = %+v, 不符合预期", c.name, ep)
		}
	}
}

func TestChatUnknown(t *testing.T) {
	if _, err := Chat("openai"); err == nil {
		t.Fatal("未知 provider 应报错")
	}
}

func TestEmbeddingKnown(t *testing.T) {
	cases := []struct{ name, model string }{
		{"qwen", "text-embedding-v4"},
		{"glm", "embedding-3"},
		{"siliconflow", "BAAI/bge-m3"},
	}
	for _, c := range cases {
		ep, err := Embedding(c.name)
		if err != nil {
			t.Fatalf("Embedding(%q) 不应报错: %v", c.name, err)
		}
		if ep.DefaultEmbeddingModel != c.model {
			t.Errorf("Embedding(%q).DefaultEmbeddingModel = %q, want %q", c.name, ep.DefaultEmbeddingModel, c.model)
		}
	}
}

func TestEmbeddingDeepSeekRejected(t *testing.T) {
	if _, err := Embedding("deepseek"); err == nil {
		t.Fatal("deepseek 无 embedding 接口，应报错")
	}
}

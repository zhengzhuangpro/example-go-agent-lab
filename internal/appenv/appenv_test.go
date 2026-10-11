package appenv

import (
	"strings"
	"testing"
)

func TestLookupFound(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	v, err := Lookup("DEEPSEEK_API_KEY")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if v != "sk-test" {
		t.Fatalf("got %q, want %q", v, "sk-test")
	}
}

func TestLookupMissing(t *testing.T) {
	_, err := Lookup("SOME_MISSING_KEY_XYZ")
	if err == nil {
		t.Fatal("缺失时应返回错误")
	}
	if !strings.Contains(err.Error(), "SOME_MISSING_KEY_XYZ") {
		t.Fatalf("错误信息应包含环境变量名，got: %v", err)
	}
}

func TestResolveEnvWins(t *testing.T) {
	// 环境变量优先于本地配置文件的 key
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-env")
	v, err := Resolve("DEEPSEEK_API_KEY", "sk-from-local")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if v != "sk-from-env" {
		t.Fatalf("got %q, want %q（env 应优先）", v, "sk-from-env")
	}
}

func TestResolveFallbackToLocal(t *testing.T) {
	// 环境变量缺失时回落到本地配置文件的 key
	t.Setenv("DEEPSEEK_API_KEY", "")
	v, err := Resolve("DEEPSEEK_API_KEY", "sk-from-local")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if v != "sk-from-local" {
		t.Fatalf("got %q, want %q（应回落 local）", v, "sk-from-local")
	}
}

func TestResolveMissing(t *testing.T) {
	t.Setenv("SOME_MISSING_KEY_XYZ", "")
	_, err := Resolve("SOME_MISSING_KEY_XYZ", "")
	if err == nil {
		t.Fatal("两处都没有 key 时应返回错误")
	}
	if !strings.Contains(err.Error(), "SOME_MISSING_KEY_XYZ") {
		t.Fatalf("错误信息应包含环境变量名，got: %v", err)
	}
	if !strings.Contains(err.Error(), "config.local.yaml") {
		t.Fatalf("错误信息应提示本地配置文件, got: %v", err)
	}
}

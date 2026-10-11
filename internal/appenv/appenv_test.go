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

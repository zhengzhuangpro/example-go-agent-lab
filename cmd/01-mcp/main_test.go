package main

import (
	"strings"
	"testing"
)

func TestTimeIn(t *testing.T) {
	s, err := timeIn("Asia/Shanghai")
	if err != nil {
		t.Fatalf("合法时区不应报错: %v", err)
	}
	if !strings.Contains(s, "CST") {
		t.Errorf("Asia/Shanghai 应渲染 CST 缩写, got %q", s)
	}
	if _, err := timeIn("Mars/Phobos"); err == nil {
		t.Error("非法时区应报错")
	}
	if _, err := timeIn(""); err != nil {
		t.Errorf("空时区应回退本地时区: %v", err)
	}
}

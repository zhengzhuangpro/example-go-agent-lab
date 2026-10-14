package main

import (
	"context"
	"strings"
	"testing"

	"github.com/philippgille/chromem-go"
)

func TestChunk(t *testing.T) {
	if got := chunk("", 500, 80); len(got) != 0 {
		t.Errorf("空文本应无块, got %d", len(got))
	}
	if got := chunk("hello", 500, 80); len(got) != 1 || got[0] != "hello" {
		t.Errorf("短文本应整块返回: %v", got)
	}
	got := chunk(strings.Repeat("字", 1200), 500, 80)
	if len(got) != 3 { // 步长 420：起点 0/420/840
		t.Fatalf("got %d 块, want 3", len(got))
	}
	for i, c := range got {
		if r := len([]rune(c)); r > 500 {
			t.Errorf("块 %d 超长: %d rune", i, r)
		}
	}
	// 相邻块应重叠 80 rune：块 2 的前 80 字 == 块 1 的后 80 字
	r0, r1 := []rune(got[0]), []rune(got[1])
	if string(r1[:80]) != string(r0[len(r0)-80:]) {
		t.Error("相邻块应重叠 80 rune")
	}
}

// 空库检索必须安静地返回空，不能触发 chromem 的 nResults>Count 报错。
func TestRetrieveEmptyCollection(t *testing.T) {
	db := chromem.NewDB() // 内存库，测试不碰磁盘
	col, err := db.GetOrCreateCollection("notes", nil, fakeEmbed)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := retrieve(context.Background(), col, "任意问题")
	if err != nil {
		t.Fatalf("空库应返回空结果而非报错: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("空库应无结果, got %d", len(rs))
	}
}

// fakeEmbed 确定性假向量（按 rune 累加），测试不调真实 embedding API。
var fakeEmbed = chromem.EmbeddingFunc(func(ctx context.Context, text string) ([]float32, error) {
	v := make([]float32, 8)
	for i, r := range text {
		v[i%8] += float32(r)
	}
	return v, nil
})

package splitter

import (
	"reflect"
	"testing"
)

func TestSplitExactChunks(t *testing.T) {
	// 10 个字符，窗口 5，重叠 1，步长 4：[0:5] [4:9] [8:10]
	got, err := Split("0123456789", 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"01234", "45678", "89"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitSingleChunkWhenShort(t *testing.T) {
	got, err := Split("短文本", 500, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"短文本"}) {
		t.Errorf("got %v", got)
	}
}

func TestSplitChineseRunes(t *testing.T) {
	// 按 rune 而非字节切：8 个汉字，窗口 4，重叠 1
	got, err := Split("一二三四五六七八", 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"一二三四", "四五六七", "七八"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitEmpty(t *testing.T) {
	got, err := Split("", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("空文本应返回 nil, got %v", got)
	}
}

func TestSplitInvalidArgs(t *testing.T) {
	for _, c := range [][2]int{{0, 0}, {-1, 0}, {5, -1}, {5, 5}, {3, 10}} {
		if _, err := Split("abc", c[0], c[1]); err == nil {
			t.Errorf("Split(size=%d, overlap=%d) 应报错", c[0], c[1])
		}
	}
}

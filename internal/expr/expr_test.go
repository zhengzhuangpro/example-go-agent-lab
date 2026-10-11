package expr

import (
	"math"
	"testing"
)

func TestEvaluateValid(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"1+2*3", 7},
		{"(1+2)*3", 9},
		{"10/4", 2.5},
		{"-(3+4)", -7},
		{"2.5*4", 10},
		{"1 + 2 * (3 - 1) / 2", 3},
		{"0.1+0.2", 0.3},
		{"((2))", 2},
		{"  7  ", 7},
	}
	for _, c := range cases {
		got, err := Evaluate(c.input)
		if err != nil {
			t.Errorf("Evaluate(%q) 不应报错: %v", c.input, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Evaluate(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestEvaluateInvalid(t *testing.T) {
	for _, input := range []string{
		"",      // 空
		"1+",    // 缺右操作数
		"(1+2",  // 缺右括号
		"3$5",   // 非法字符
		"1/0",   // 除零
		"2 3",   // 多余 token
		"1.2.3", // 非法数字
	} {
		if _, err := Evaluate(input); err == nil {
			t.Errorf("Evaluate(%q) 应报错", input)
		}
	}
}

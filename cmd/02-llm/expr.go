// expr.go：安全的四则运算求值器（复制自 demo01——教学项目宁复制，不抽象）。
// 手写 tokenize + 递归下降解析——非法输入只会得到明确报错，不可能执行任意代码。
package main

import (
	"fmt"
	"strconv"
	"unicode"
)

// evaluate 计算表达式：支持 + - * /、括号、一元负号、整数与小数。
func evaluate(input string) (float64, error) {
	toks, err := tokenize(input)
	if err != nil {
		return 0, err
	}
	p := &parser{toks: toks}
	val, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if tail := p.peek(); tail.kind != tokEOF {
		return 0, fmt.Errorf("表达式末尾有多余内容 %q", tail.text)
	}
	return val, nil
}

type tokenKind int

const (
	tokNumber tokenKind = iota
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokLParen
	tokRParen
	tokEOF
)

type token struct {
	kind tokenKind
	text string
}

// tokenize 把源串切成 token 流；跳过空白，拒绝任何非法字符。
func tokenize(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r >= '0' && r <= '9' || r == '.':
			start, dots := i, 0
			for i < len(rs) {
				c := rs[i]
				if c >= '0' && c <= '9' {
					i++
				} else if c == '.' {
					dots++
					i++
				} else {
					break
				}
			}
			text := string(rs[start:i])
			if dots > 1 {
				return nil, fmt.Errorf("非法数字 %q", text)
			}
			toks = append(toks, token{kind: tokNumber, text: text})
		case r == '+':
			toks = append(toks, token{kind: tokPlus, text: "+"})
			i++
		case r == '-':
			toks = append(toks, token{kind: tokMinus, text: "-"})
			i++
		case r == '*':
			toks = append(toks, token{kind: tokStar, text: "*"})
			i++
		case r == '/':
			toks = append(toks, token{kind: tokSlash, text: "/"})
			i++
		case r == '(':
			toks = append(toks, token{kind: tokLParen, text: "("})
			i++
		case r == ')':
			toks = append(toks, token{kind: tokRParen, text: ")"})
			i++
		default:
			return nil, fmt.Errorf("无法识别的字符 %q", string(r))
		}
	}
	toks = append(toks, token{kind: tokEOF})
	return toks, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{kind: tokEOF}
}

func (p *parser) next() token {
	t := p.peek()
	p.pos++
	return t
}

// 文法（优先级从低到高）：
//
//	expr   = term (('+'|'-') term)*
//	term   = factor (('*'|'/') factor)*
//	factor = NUMBER | '(' expr ')' | '-' factor

func (p *parser) parseExpr() (float64, error) {
	val, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek().kind {
		case tokPlus:
			p.next()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val += r
		case tokMinus:
			p.next()
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			val -= r
		default:
			return val, nil
		}
	}
}

func (p *parser) parseTerm() (float64, error) {
	val, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek().kind {
		case tokStar:
			p.next()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			val *= r
		case tokSlash:
			p.next()
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("除数为零")
			}
			val /= r
		default:
			return val, nil
		}
	}
}

func (p *parser) parseFactor() (float64, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return strconv.ParseFloat(t.text, 64)
	case tokLParen:
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.next().kind != tokRParen {
			return 0, fmt.Errorf("缺少右括号")
		}
		return val, nil
	case tokMinus:
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	default:
		return 0, fmt.Errorf("意外的符号 %q", t.text)
	}
}

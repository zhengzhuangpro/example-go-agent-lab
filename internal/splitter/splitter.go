// Package splitter 实现最简单的教学切分策略：固定 rune 窗口 + 重叠。
// 按 rune 计数保证中文不被从字节中间切断；
// overlap 让相邻块共享一段文本，防止关键句正好落在块边界被切断。
package splitter

import "fmt"

// Split 把 text 切成若干块：每块至多 chunkSize 个 rune，
// 相邻块重叠 overlap 个 rune。空文本返回 nil。
func Split(text string, chunkSize, overlap int) ([]string, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize 必须为正数, got %d", chunkSize)
	}
	if overlap < 0 || overlap >= chunkSize {
		return nil, fmt.Errorf("overlap 必须满足 0 <= overlap < chunkSize, got overlap=%d chunkSize=%d", overlap, chunkSize)
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, nil
	}
	step := chunkSize - overlap
	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks, nil
}

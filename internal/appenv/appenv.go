// Package appenv 提供环境变量读取：API key 一律从环境变量来，绝不落盘。
package appenv

import (
	"fmt"
	"os"
)

// Lookup 读取环境变量；缺失时返回指明变量名的错误，调用方 log.Fatal 即可。
func Lookup(envName string) (string, error) {
	v := os.Getenv(envName)
	if v == "" {
		return "", fmt.Errorf("缺少环境变量 %s（请先设置，例如: export %s=sk-xxx）", envName, envName)
	}
	return v, nil
}

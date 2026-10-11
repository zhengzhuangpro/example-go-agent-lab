// Package appenv 提供 API key 的解析：优先环境变量，回落到本地覆盖文件，
// 被提交的文件中绝不出现真实 key。
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

// Resolve 按优先级解析一个 API key：环境变量 > 本地覆盖文件里的值（configured）。
// 两处都没有时返回的错误同时指明变量名与本地文件的配置方法。
func Resolve(envName, configured string) (string, error) {
	if v := os.Getenv(envName); v != "" {
		return v, nil
	}
	if configured != "" {
		return configured, nil
	}
	return "", fmt.Errorf("缺少 %s 的 API key：请设置环境变量 %s，或在 configs/config.local.yaml（已 gitignore）中配置 api_keys.%s", envName, envName, envName)
}

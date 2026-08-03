package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/mapleafgo/singcast/translator"
)

// loadConfigJSON 读取配置文件并转换为 sing-box JSON。
// JSON 输入直接透传（可能先做 base64 解码）；YAML 输入经翻译器转换，
// 翻译产生的 warning 打到 stderr（非致命，但用户需要知道哪些配置被降级或跳过）。
// ruleSetProxy 为空表示不改写 rule-set 下载地址。
// 文件不存在、内容为空或翻译失败时返回 error。
func loadConfigJSON(path, ruleSetProxy string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", fmt.Errorf("config file is empty")
	}

	result, warnings, err := translator.Convert(data)
	if err != nil {
		return "", fmt.Errorf("translate config: %w", err)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if ruleSetProxy != "" {
		var err error
		result, err = translator.ApplyRuleSetProxy(result, ruleSetProxy)
		if err != nil {
			return "", fmt.Errorf("apply rule-set proxy: %w", err)
		}
	}
	return result, nil
}

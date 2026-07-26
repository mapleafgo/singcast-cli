package translator

import (
	"encoding/json"
)

// Format 标识输入配置的格式。
type Format int

const (
	// FormatJSON 表示 sing-box 原生 JSON 配置，无需翻译。
	FormatJSON Format = iota
	// FormatYAML 表示 Clash Meta (mihomo) YAML 配置，需经翻译器转换。
	FormatYAML
)

// DetectFormat 判断配置内容是 sing-box JSON 还是 mihomo YAML。
//
// 判定方式是尝试按 JSON 解析：仅当解析成功且顶层为对象时才算 JSON。
// 非对象的合法 JSON（数组、裸字符串、数字）会被判为 YAML——这是有意的：
// 两种格式的配置顶层都必须是映射，非对象输入本就不是有效配置，
// 交给 YAML 分支能得到更贴近用户意图的报错。
func DetectFormat(data []byte) Format {
	var v any
	if json.Unmarshal(data, &v) == nil {
		if _, ok := v.(map[string]any); ok {
			return FormatJSON
		}
	}
	return FormatYAML
}

package translator

import (
	"encoding/json"
)

type Format int

const (
	FormatJSON Format = iota
	FormatYAML
)

func DetectFormat(data []byte) Format {
	var v any
	if json.Unmarshal(data, &v) == nil {
		return FormatJSON
	}
	return FormatYAML
}

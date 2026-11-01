// Package jsonx 提供对「任意 JSON」的宽松读取辅助。
//
// OpenList 的响应字段随驱动/版本变化，强行建 struct 会脆。
// 这里统一用 map[string]any + json.Number 表达，读写都不丢精度。
package jsonx

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// Decode 解析 JSON，数字保留为 json.Number（避免大整数被 float64 截断）。
func Decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeString 同上，输入是字符串。
func DecodeString(s string) (any, error) { return Decode([]byte(s)) }

// Encode 序列化为紧凑 JSON，不转义 < > &。
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Object 把 v 当对象读；不是对象时返回 nil。
func Object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// List 把 v 当数组读；不是数组时返回 nil。
func List(v any) []any {
	l, _ := v.([]any)
	return l
}

// Objects 把数组里的对象挑出来。
func Objects(v any) []map[string]any {
	out := make([]map[string]any, 0, 4)
	for _, it := range List(v) {
		if m := Object(it); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// Str 读字符串字段；数字/布尔按字符串返回，缺失返回 ""。
func Str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	return StrOf(m[key])
}

// StrOf 把任意值转成字符串。
func StrOf(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// Bool 读布尔字段，字段缺失或类型不符时按 false 处理。
func Bool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	return BoolOf(m[key])
}

// BoolOf 宽松判断真假。
func BoolOf(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case json.Number:
		f, err := t.Float64()
		return err == nil && f != 0
	case float64:
		return t != 0
	}
	return false
}

// Int 读整数字段，缺失/非法返回 0。
func Int(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	return IntOf(m[key])
}

// IntOf 宽松转 int64。
func IntOf(v any) int64 {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return int64(f)
		}
	case float64:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	case string:
		if i, err := strconv.ParseInt(t, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

// Float 读浮点字段，第二个返回值表示是否可当数字。
func Float(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	return FloatOf(m[key])
}

// FloatOf 宽松转 float64。
func FloatOf(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	}
	return 0, false
}

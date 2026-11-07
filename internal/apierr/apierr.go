// Package apierr 定义各服务共用的接口错误与网络错误。
package apierr

import (
	"fmt"
	"strconv"
	"strings"
)

// Hints 是各服务给自己的错误码补充的下一步提示。
// 空字段表示该状态码不额外提示。
type Hints struct {
	Unauthorized string
	Forbidden    string
	Unsupported  string
}

// ApiError 表示服务端返回 code != 200，或返回了非 JSON。
type ApiError struct {
	Code     *int
	Message  string
	Endpoint string
	Hints    Hints
}

func (e *ApiError) Error() string { return e.Render() }

// Render 生成带下一步提示的错误文案。
func (e *ApiError) Render() string {
	hint := ""
	if e.Code != nil {
		switch *e.Code {
		case 401:
			hint = e.Hints.Unauthorized
		case 403:
			hint = e.Hints.Forbidden
		case 404:
			msg := strings.ToLower(e.Message)
			switch {
			case msg == "" || containsAny(msg, "object", "path", "file", "目录", "文件"):
				hint = "\n  路径不存在"
			case containsAny(msg, "not found", "not exist", "不存在"):
				hint = "" // 资源本身不存在（如 driver not found），message 已说明
			default:
				hint = "\n  该功能或资源在此存储上不可用"
			}
		case 405:
			hint = e.Hints.Unsupported
		}
	}

	message := e.Message
	if message == "" {
		message = "未知错误"
	}
	code := "None"
	if e.Code != nil {
		code = strconv.Itoa(*e.Code)
	}
	head := fmt.Sprintf("[%s] %s", code, message)
	if e.Endpoint != "" {
		head += "  (" + e.Endpoint + ")"
	}
	return head + hint
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// NetworkError 表示连不上、超时等传输层错误。
type NetworkError struct {
	Msg string
}

func (e *NetworkError) Error() string { return e.Msg }

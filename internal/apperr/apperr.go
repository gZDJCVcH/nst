// Package apperr 定义退出码错误与统一的错误分类。
package apperr

import (
	"context"
	"errors"
	"fmt"

	"nst/internal/apierr"
	"nst/internal/config"
	"nst/internal/pathx"
)

// ExitError 携带退出码与要打印的消息，由 App.Execute 统一处理。
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

// Exitf 构造带退出码的错误。
func Exitf(code int, format string, args ...any) error {
	return &ExitError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// CodeOf 把错误映射为进程退出码。
func CodeOf(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	var pe *pathx.PathError
	if errors.As(err, &pe) {
		return 2
	}
	var ce *config.ConfigError
	if errors.As(err, &ce) {
		return 2
	}
	var ae *apierr.ApiError
	if errors.As(err, &ae) {
		return 3
	}
	var ne *apierr.NetworkError
	if errors.As(err, &ne) {
		return 4
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

package fault

import (
	"context"
	"errors"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string       { return e.Code + ": " + e.Message }
func New(code, message string) error { return &Error{Code: code, Message: message} }
func Is(err error, code string) bool { var e *Error; return errors.As(err, &e) && e.Code == code }

// Public prevents driver, filesystem and remote errors leaking to clients.
func Public(err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: "DEADLINE_EXCEEDED", Message: "操作超时"}
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "CANCELED", Message: "操作已取消"}
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: "INTERNAL", Message: "服务暂时不可用"}
}

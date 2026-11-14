package fault

import "errors"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string       { return e.Code + ": " + e.Message }
func New(code, message string) error { return &Error{Code: code, Message: message} }
func Is(err error, code string) bool { var e *Error; return errors.As(err, &e) && e.Code == code }

// Public prevents driver, filesystem and remote errors leaking to clients.
func Public(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: "INTERNAL", Message: "服务暂时不可用"}
}

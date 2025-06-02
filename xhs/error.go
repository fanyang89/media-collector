package xhs

import "fmt"

type Error struct {
	Code    int
	Message string
}

func newError(code int, msg string) *Error {
	return &Error{code, msg}
}

func (e *Error) Error() string {
	return fmt.Sprintf("xhs error code: %d, message: %s", e.Code, e.Message)
}

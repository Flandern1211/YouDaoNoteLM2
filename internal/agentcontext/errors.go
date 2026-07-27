package agentcontext

import "fmt"

// ErrorCode 错误代码
type ErrorCode string

const (
	ErrCodeInvalidConfig         ErrorCode = "INVALID_CONFIG"
	ErrCodeDuplicateKey          ErrorCode = "DUPLICATE_KEY"
	ErrCodeProfileNotFound       ErrorCode = "PROFILE_NOT_FOUND"
	ErrCodeModelUnknown          ErrorCode = "MODEL_UNKNOWN"
	ErrCodeProviderExhausted     ErrorCode = "PROVIDER_EXHAUSTED"
	ErrCodeHardBudgetExceeded    ErrorCode = "HARD_BUDGET_EXCEEDED"
	ErrCodeInvalidInput          ErrorCode = "INVALID_INPUT"
	ErrCodeInvalidTurnHandle     ErrorCode = "INVALID_TURN_HANDLE"
	ErrCodeTokenCountUnavailable ErrorCode = "TOKEN_COUNT_UNAVAILABLE"
)

// Error 上下文管理错误
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is/As
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewError 创建新错误
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewErrorWithCause 创建带原因的错误
func NewErrorWithCause(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// IsErrorCode 检查错误是否为指定代码
func IsErrorCode(err error, code ErrorCode) bool {
	if e, ok := err.(*Error); ok {
		return e.Code == code
	}
	return false
}

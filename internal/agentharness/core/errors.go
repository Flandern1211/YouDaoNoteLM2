package core

import "errors"

var (
	// ErrRunNotFound Run 不存在。
	ErrRunNotFound = errors.New("run not found")
	// ErrRunAlreadyExists Run 已存在。
	ErrRunAlreadyExists = errors.New("run already exists")
	// ErrAuthorityStale Authority 不匹配，CAS 失败。
	ErrAuthorityStale = errors.New("authority stale")
	// ErrInvalidTransition 状态转换无效。
	ErrInvalidTransition = errors.New("invalid transition")
	// ErrStepNotFound Step 不存在。
	ErrStepNotFound = errors.New("step not found")
	// ErrStepAlreadyExists Step 已存在。
	ErrStepAlreadyExists = errors.New("step already exists")
	// ErrAttemptNotFound Attempt 不存在。
	ErrAttemptNotFound = errors.New("attempt not found")
	// ErrInvalidErrorClass 未知的错误分类。
	ErrInvalidErrorClass = errors.New("invalid error class")
	// ErrNotQueued Run 不在 queued 状态，无法 claim。
	ErrNotQueued = errors.New("run not in queued state")
)

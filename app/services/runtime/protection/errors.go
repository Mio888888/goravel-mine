package protection

import "errors"

const (
	RejectionNone               = ""
	RejectionRateLimited        = "RATE_LIMITED"
	RejectionCircuitOpen        = "CIRCUIT_OPEN"
	RejectionConcurrencyLimited = "CONCURRENCY_LIMITED"
)

var (
	ErrRateLimited        = errors.New("protection rate limited")
	ErrCircuitOpen        = errors.New("protection circuit open")
	ErrConcurrencyLimited = errors.New("protection concurrency limited")
	ErrDependencyTimeout  = errors.New("protected dependency timeout")
	ErrDependencyFailure  = errors.New("protected dependency failure")
)

type Error struct {
	Kind     string
	Resource string
}

func (e Error) Error() string {
	switch e.Kind {
	case RejectionRateLimited:
		return "请求已被限流规则拒绝"
	case RejectionCircuitOpen:
		return "请求已被熔断规则拒绝"
	case RejectionConcurrencyLimited:
		return "请求已被并发隔离规则拒绝"
	default:
		return "请求已被服务保护规则拒绝"
	}
}

func (e Error) Unwrap() error {
	switch e.Kind {
	case RejectionRateLimited:
		return ErrRateLimited
	case RejectionCircuitOpen:
		return ErrCircuitOpen
	case RejectionConcurrencyLimited:
		return ErrConcurrencyLimited
	default:
		return nil
	}
}

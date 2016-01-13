package middleware

import (
	"fmt"
	"time"
)

const (
	CircuitBreakerType = "cbreaker"
	CircuitBreakerID   = "cb1"
)

// CircuitBreaker is a spec for the respective vulcan's middleware that lets vulcan to fallback to
// some default response and trigger some action when an erroneous condition on a location is met.
type CircuitBreaker struct {
	Condition        string        `json:"Condition"`
	Fallback         string        `json:"Fallback"`
	CheckPeriod      time.Duration `json:"CheckPeriod"`
	FallbackDuration time.Duration `json:"FallbackDuration"`
	RecoveryDuration time.Duration `json:"RecoveryDuration"`
	OnTripped        string        `json:"OnTripped"`
	OnStandby        string        `json:"OnStandby"`
}

func NewCircuitBreaker(spec CircuitBreaker) Middleware {
	return Middleware{
		Type:     CircuitBreakerType,
		ID:       CircuitBreakerID,
		Priority: DefaultPriority,
		Spec:     spec,
	}
}

func (cb CircuitBreaker) String() string {
	return fmt.Sprintf("CircuitBreaker(Condition=%v, Fallback=%v, CheckPeriod=%v, FallbackDuration=%v, RecoveryDuration=%v, OnTripped=%v, OnStandby=%v)",
		cb.Condition, cb.Fallback, cb.CheckPeriod, cb.FallbackDuration, cb.RecoveryDuration, cb.OnTripped, cb.OnStandby)
}

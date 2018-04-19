package auth

import (
	"errors"
	"fmt"
)

type AuthQuota struct {
	Limit int `json:"limit"`
	InUse int `json:"inuse"`
}

type AuthQuotaService interface {
	ReserveApp(email string, quota *AuthQuota) error
	ReleaseApp(email string, quota *AuthQuota) error
	ChangeLimit(email string, quota *AuthQuota, limit int) error
	FindByUserEmail(email string) (*AuthQuota, error)
}

type AuthQuotaStorage interface {
	IncInUse(email string, quota *AuthQuota, quantity int) error
	SetLimit(email string, quota *AuthQuota, limit int) error
	FindByUserEmail(email string) (*AuthQuota, error)
}

type AuthQuotaExceededError struct {
	Requested uint
	Available uint
}

func (err *AuthQuotaExceededError) Error() string {
	return fmt.Sprintf("Quota exceeded. Available: %d, Requested: %d.", err.Available, err.Requested)
}

var (
	ErrCantRelease             = errors.New("Cannot release unreserved app.")
	ErrLimitLowerThanAllocated = errors.New("new limit is lesser than the current allocated value")
)

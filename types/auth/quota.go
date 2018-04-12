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
	ReserveApp(user *User, quota *AuthQuota) error
	ReleaseApp(user *User, quota *AuthQuota) error
	CheckUser(user *User, quota *AuthQuota) error
	ChangeQuota(user *User, limit int) error
}

type AuthQuotaStorage interface {
	IncInUse(user *User, quota *AuthQuota, quantity int) error
	SetLimit(user *User, quota *AuthQuota, limit int) error
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

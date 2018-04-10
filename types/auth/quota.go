package auth

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
	IncInUse(user *User, quota *AuthQuota) error
	SetLimit(user *User, quota *AuthQuota) error
}

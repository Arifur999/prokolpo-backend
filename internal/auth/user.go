// Package auth signs people up and in: registration, login, rotating refresh
// sessions, and password reset by emailed link.
package auth

import (
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// User is an account row.
type User struct {
	ID           string
	FirstName    string
	LastName     string
	CompanyName  *string
	Phone        string
	Email        *string
	PasswordHash string
	Locale       string
	Status       string
	StaffRole    *string
	CreatedAt    time.Time
}

const statusSuspended = "suspended"

// IsSuspended reports whether the account is blocked from signing in.
func (u User) IsSuspended() bool { return u.Status == statusSuspended }

// Role is what goes in the access token: staff can open the admin panel.
func (u User) Role() string {
	if u.StaffRole != nil {
		return token.RoleStaff
	}
	return token.RoleUser
}

// UserView is the account as the API shows it. The password hash never leaves the package.
type UserView struct {
	ID          string    `json:"id"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	Name        string    `json:"name"`
	CompanyName *string   `json:"companyName"`
	Phone       string    `json:"phone"`
	Email       *string   `json:"email"`
	Locale      string    `json:"locale"`
	Role        string    `json:"role"`
	StaffRole   *string   `json:"staffRole"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (u User) view() UserView {
	return UserView{
		ID: u.ID, FirstName: u.FirstName, LastName: u.LastName, Name: u.FirstName + " " + u.LastName,
		CompanyName: u.CompanyName, Phone: u.Phone, Email: u.Email, Locale: u.Locale,
		Role: u.Role(), StaffRole: u.StaffRole, CreatedAt: u.CreatedAt,
	}
}

// Session is what a successful register, login or refresh returns.
type Session struct {
	User                  UserView  `json:"user"`
	AccessToken           string    `json:"accessToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshToken          string    `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

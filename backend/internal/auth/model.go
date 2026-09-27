package auth

import "errors"

type User struct {
	ID       string `json:"id" format:"uuid" example:"11111111-1111-4111-8111-111111111111" binding:"required"`
	Username string `json:"username" example:"demo" binding:"required"`
}

type LoginRequest struct {
	// Case-sensitive username, at most 128 UTF-8 bytes.
	Username string `json:"username" example:"demo" minLength:"1" maxLength:"128" binding:"required"`
	// Password length must be 1–72 UTF-8 bytes.
	Password string `json:"password" example:"DemoPass123!" format:"password" minLength:"1" maxLength:"72" binding:"required"`
}

type UserResponse struct {
	User User `json:"user" binding:"required"`
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrInvalidInput       = errors.New("invalid login input")
)

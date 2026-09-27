package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const SessionLifetime = 24 * time.Hour

// Unknown users still incur a bcrypt comparison at the seed accounts' cost.
const dummyHash = "$2a$12$rulRn4vyeGnqXoQApQ1ByerTX/ffr/ggu2oylSavGUA3slAo72.rW"

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{pool: pool, now: now}
}

func (s *Service) Login(ctx context.Context, username, password, previousToken string) (User, string, error) {
	if strings.TrimSpace(username) == "" || len(username) > 128 || len(password) == 0 || len(password) > 72 {
		return User{}, "", ErrInvalidInput
	}
	var user User
	var hash string
	err := s.pool.QueryRow(ctx, "SELECT id::text, username, password_hash FROM users WHERE username=$1", username).Scan(&user.ID, &user.Username, &hash)
	missing := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !missing {
		return User{}, "", err
	}
	if missing {
		hash = dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if missing || errors.Is(passwordErr, bcrypt.ErrMismatchedHashAndPassword) {
		return User{}, "", ErrInvalidCredentials
	}
	if passwordErr != nil {
		return User{}, "", passwordErr
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return User{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback(ctx)
	now := s.now().UTC()
	if _, err = tx.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1 OR expires_at <= $2", tokenHash(previousToken), now); err != nil {
		return User{}, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_sessions(user_id,token_hash,created_at,expires_at) VALUES($1,$2,$3,$4)`, user.ID, tokenHash(token), now, now.Add(SessionLifetime)); err != nil {
		return User{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if len(token) != 43 {
		return User{}, ErrUnauthenticated
	}
	var user User
	err := s.pool.QueryRow(ctx, `SELECT u.id::text,u.username FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>$2`, tokenHash(token), s.now().UTC()).Scan(&user.ID, &user.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	return user, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1", tokenHash(token))
	return err
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

type userContextKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

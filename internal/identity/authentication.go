package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type LoginResult struct {
	User      User
	Token     string `json:"-"`
	ExpiresAt time.Time
}

func (s *Service) Login(ctx context.Context, input Credentials) (LoginResult, error) {
	// Login validates presence and the upper bound; strength is enforced at registration.
	input, err := validateCredentials(input, 1)
	if err != nil {
		return LoginResult{}, err
	}
	if ctx.Err() != nil {
		return LoginResult{}, ErrIdentityUnavailable
	}
	if !s.beginPasswordWork() {
		return LoginResult{}, ErrCredentialBusy
	}
	defer s.endPasswordWork()
	readCtx, cancelRead := context.WithTimeout(ctx, 5*time.Second)
	var user User
	var verifier string
	err = s.pool.QueryRow(readCtx, `
		SELECT id, email, password_hash, created_at, updated_at
		FROM biznes.users WHERE email = $1`, input.Email).
		Scan(&user.ID, &user.Email, &verifier, &user.CreatedAt, &user.UpdatedAt)
	cancelRead()
	if err != nil && !errors.Is(err, pgx.ErrNoRows) || ctx.Err() != nil {
		return LoginResult{}, ErrIdentityUnavailable
	}
	if !passwordMatches(input.Password, verifier) {
		return LoginResult{}, ErrUnauthenticated
	}
	if ctx.Err() != nil {
		return LoginResult{}, ErrIdentityUnavailable
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return LoginResult{}, errors.New("generate session token")
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256(secret)
	writeCtx, cancelWrite := context.WithTimeout(ctx, 5*time.Second)
	defer cancelWrite()
	result := LoginResult{User: user, Token: token}
	err = s.pool.QueryRow(writeCtx, `
		INSERT INTO biznes.sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + interval '8 hours')
		RETURNING expires_at`, digest[:], user.ID).Scan(&result.ExpiresAt)
	if err != nil {
		return LoginResult{}, ErrIdentityUnavailable
	}
	return result, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	digest, ok := sessionDigest(token)
	if !ok {
		return User{}, ErrUnauthenticated
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var user User
	// ponytail: each authorized request touches PostgreSQL; optimize idle refresh only if measured traffic requires it.
	err := s.pool.QueryRow(checkCtx, `
		WITH touched AS (
			UPDATE biznes.sessions SET last_seen_at = clock_timestamp()
			WHERE token_hash = $1 AND expires_at > clock_timestamp()
			  AND last_seen_at > clock_timestamp() - interval '15 minutes'
			RETURNING user_id
		)
		SELECT u.id, u.email, u.created_at, u.updated_at
		FROM biznes.users u JOIN touched t ON t.user_id = u.id`, digest[:]).
		Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, ErrIdentityUnavailable
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	digest, ok := sessionDigest(token)
	if !ok {
		return ErrUnauthenticated
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(deleteCtx, `DELETE FROM biznes.sessions WHERE token_hash = $1`, digest[:]); err != nil {
		return ErrIdentityUnavailable
	}
	return nil
}

func sessionDigest(token string) ([32]byte, bool) {
	if len(token) != 43 {
		return [32]byte{}, false
	}
	secret, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(secret) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(secret), true
}

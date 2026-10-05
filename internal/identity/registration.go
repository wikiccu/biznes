package identity

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCredentialBusy      = errors.New("credential limit reached")
	ErrEmailInUse          = errors.New("email already in use")
	ErrIdentityUnavailable = errors.New("identity storage unavailable")
	ErrUnauthenticated     = errors.New("invalid credentials or session")
)

type Credentials struct {
	Email    string
	Password string
}

type FieldError struct {
	Field string
	Code  string
}

type ValidationError struct {
	Fields []FieldError
}

func (*ValidationError) Error() string { return "invalid credentials input" }

// Service manages global identities and sessions; organization access belongs to memberships.
type Service struct {
	pool      *pgxpool.Pool
	mu        sync.Mutex
	active    bool
	nextStart time.Time
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func validateCredentials(input Credentials, minimumPasswordLength int) (Credentials, error) {
	input.Email = strings.TrimSpace(input.Email)
	var fields []FieldError
	if input.Email == "" {
		fields = append(fields, FieldError{"email", "required"})
	} else if !validEmail(input.Email) {
		fields = append(fields, FieldError{"email", "invalid_format"})
	}
	if input.Password == "" {
		fields = append(fields, FieldError{"password", "required"})
	} else if !utf8.ValidString(input.Password) {
		fields = append(fields, FieldError{"password", "invalid_format"})
	} else if length := utf8.RuneCountInString(input.Password); length < minimumPasswordLength || length > 128 {
		fields = append(fields, FieldError{"password", "out_of_range"})
	}
	if len(fields) != 0 {
		return Credentials{}, &ValidationError{Fields: fields}
	}
	input.Email = strings.ToLower(input.Email)
	return input, nil
}

func (r *Service) beginPasswordWork() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	// ponytail: one password operation and one start/second per process; add trusted-ingress client limits before public scale.
	if r.active || time.Now().Before(r.nextStart) {
		return false
	}
	r.active = true
	r.nextStart = time.Now().Add(time.Second)
	return true
}

func (r *Service) endPasswordWork() {
	r.mu.Lock()
	r.active = false
	r.mu.Unlock()
}

func (r *Service) Register(ctx context.Context, input Credentials) (User, error) {
	input, err := validateCredentials(input, 15)
	if err != nil {
		return User{}, err
	}
	if ctx.Err() != nil {
		return User{}, ErrIdentityUnavailable
	}
	if !r.beginPasswordWork() {
		return User{}, ErrCredentialBusy
	}
	defer r.endPasswordWork()
	hash, err := hashPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	// Hashing has fixed costs; cancellation prevents subsequent persistence.
	if ctx.Err() != nil {
		return User{}, ErrIdentityUnavailable
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var user User
	err = r.pool.QueryRow(writeCtx, `
		INSERT INTO biznes.users (email, password_hash) VALUES ($1, $2)
		RETURNING id, email, created_at, updated_at`, input.Email, hash).
		Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_unique" {
			return User{}, ErrEmailInUse
		}
		return User{}, ErrIdentityUnavailable
	}
	return user, nil
}

func validEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	for _, c := range email {
		if c < '!' || c > '~' {
			return false
		}
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" {
		return false
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || len(local) > 64 {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

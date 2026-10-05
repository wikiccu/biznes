package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

var (
	ErrRegistrationBusy        = errors.New("registration limit reached")
	ErrEmailInUse              = errors.New("email already in use")
	ErrRegistrationUnavailable = errors.New("registration storage unavailable")
)

type RegistrationInput struct {
	Email    string
	Password string
}

type FieldError struct {
	Field string
	Code  string
}

type RegistrationValidationError struct {
	Fields []FieldError
}

func (*RegistrationValidationError) Error() string { return "invalid registration input" }

// Registrar validates and creates global identities without granting a session.
type Registrar struct {
	pool      *pgxpool.Pool
	mu        sync.Mutex
	active    bool
	nextStart time.Time
}

func NewRegistrar(pool *pgxpool.Pool) *Registrar { return &Registrar{pool: pool} }

func (r *Registrar) Register(ctx context.Context, input RegistrationInput) (User, error) {
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
	} else if length := utf8.RuneCountInString(input.Password); length < 15 || length > 128 {
		fields = append(fields, FieldError{"password", "out_of_range"})
	}
	if len(fields) != 0 {
		return User{}, &RegistrationValidationError{Fields: fields}
	}
	input.Email = strings.ToLower(input.Email)
	if ctx.Err() != nil {
		return User{}, ErrRegistrationUnavailable
	}

	r.mu.Lock()
	// ponytail: one in-flight registration and one start/second per process; add gateway/client limits before public scale.
	if r.active || time.Now().Before(r.nextStart) {
		r.mu.Unlock()
		return User{}, ErrRegistrationBusy
	}
	r.active = true
	r.nextStart = time.Now().Add(time.Second)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.active = false
		r.mu.Unlock()
	}()

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return User{}, errors.New("generate password salt")
	}
	// Argon2id v19: OWASP's 19 MiB / two passes / one lane minimum.
	key := argon2.IDKey([]byte(input.Password), salt, 2, 19*1024, 1, 32)
	hash := fmt.Sprintf("$argon2id$v=%d$m=19456,t=2,p=1$%s$%s", argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
	// Hashing has fixed costs; cancellation prevents subsequent persistence.
	if ctx.Err() != nil {
		return User{}, ErrRegistrationUnavailable
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var user User
	err := r.pool.QueryRow(writeCtx, `
		INSERT INTO biznes.users (email, password_hash) VALUES ($1, $2)
		RETURNING id, email, created_at, updated_at`, input.Email, hash).
		Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_unique" {
			return User{}, ErrEmailInUse
		}
		return User{}, ErrRegistrationUnavailable
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

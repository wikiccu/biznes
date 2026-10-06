package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func userDTO(user identity.User) publicUser {
	return publicUser{user.ID, user.Email, user.CreatedAt.UTC(), user.UpdatedAt.UTC()}
}

func registration(service *identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := credentialsInput(c)
		if !ok {
			return
		}
		user, err := service.Register(c.Request.Context(), input)
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, gin.H{"data": userDTO(user)})
	}
}

func writeIdentityError(c *gin.Context, err error) {
	var validation *identity.ValidationError
	switch {
	case errors.As(err, &validation):
		details := make([]ErrorDetail, 0, len(validation.Fields))
		for _, field := range validation.Fields {
			details = append(details, ErrorDetail{Field: field.Field, Code: field.Code})
		}
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.", details...)
	case errors.Is(err, identity.ErrCredentialBusy):
		c.Header("Retry-After", "1")
		WriteError(c, http.StatusTooManyRequests, "rate_limited", "Credential service is busy. Please retry later.")
	case errors.Is(err, identity.ErrEmailInUse):
		WriteError(c, http.StatusConflict, "conflict", "Registration could not be completed.")
	case errors.Is(err, identity.ErrUnauthenticated):
		c.Header("WWW-Authenticate", `Bearer realm="biznes"`)
		WriteError(c, http.StatusUnauthorized, "unauthenticated", "Authentication is required or invalid.")
	case errors.Is(err, identity.ErrIdentityUnavailable):
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "Identity service is temporarily unavailable.")
	default:
		WriteError(c, http.StatusInternalServerError, "internal_error", "An internal error occurred.")
	}
}

func credentialsInput(c *gin.Context) (identity.Credentials, bool) {
	var input identity.Credentials
	ok := jsonStringInput(c, map[string]*string{"email": &input.Email, "password": &input.Password})
	return input, ok
}

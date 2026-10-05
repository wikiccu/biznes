package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Query parameters are not accepted.")
		return input, false
	}
	contentTypes := c.Request.Header.Values("Content-Type")
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	charset, hasCharset := params["charset"]
	if len(contentTypes) != 1 || err != nil || mediaType != "application/json" ||
		len(params) > 1 || hasCharset && !strings.EqualFold(charset, "utf-8") {
		WriteError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Use application/json with UTF-8 encoding.")
		return input, false
	}
	if len(params) == 1 && !hasCharset {
		WriteError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Use application/json with UTF-8 encoding.")
		return input, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	body, err := io.ReadAll(c.Request.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "The request body is too large.")
		return input, false
	}
	if err != nil || !utf8.Valid(body) || !validUnicodeEscapes(body) || !decodeCredentials(body, &input) {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide one JSON object with email and password string fields.")
		return input, false
	}
	return input, true
}

// Token decoding enforces exact field names and rejects duplicate keys/nulls.
func decodeCredentials(body []byte, input *identity.Credentials) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]bool, 2)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := token.(string)
		if !ok || seen[name] || name != "email" && name != "password" {
			return false
		}
		seen[name] = true
		var value *string
		if err := decoder.Decode(&value); err != nil || value == nil {
			return false
		}
		if name == "email" {
			input.Email = *value
		} else {
			input.Password = *value
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

// encoding/json replaces lone surrogate escapes; reject them instead of changing a password.
func validUnicodeEscapes(body []byte) bool {
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) || body[i] != 'u' {
			continue
		}
		if i+4 >= len(body) {
			return false
		}
		code, err := strconv.ParseUint(string(body[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(body) || body[i+1] != '\\' || body[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(body[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		} else if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
	}
	return true
}

package httpserver

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/identity"
)

type loginData struct {
	User        publicUser `json:"user"`
	AccessToken string     `json:"access_token"`
	TokenType   string     `json:"token_type"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

func login(service *identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := credentialsInput(c)
		if !ok {
			return
		}
		result, err := service.Login(c.Request.Context(), input)
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": loginData{
			User: userDTO(result.User), AccessToken: result.Token, TokenType: "Bearer", ExpiresAt: result.ExpiresAt.UTC(),
		}})
	}
}

func emptyAuthRequest(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Query parameters are not accepted.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 0)
	if _, err := io.ReadAll(c.Request.Body); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "This endpoint requires an empty request body.")
		return
	}
	c.Next()
}

func sessionAuthentication(service *identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			writeSessionUnauthorized(c)
			return
		}
		user, err := service.Authenticate(c.Request.Context(), token)
		if errors.Is(err, identity.ErrUnauthenticated) {
			writeSessionUnauthorized(c)
			return
		}
		if err != nil {
			writeIdentityError(c, err)
			return
		}
		c.Set("identity_user", user)
		c.Set("identity_token", token)
		c.Next()
	}
}

func bearerToken(c *gin.Context) (string, bool) {
	values := c.Request.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > 128 {
		return "", false
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	token = strings.TrimLeft(token, " ")
	return token, ok && strings.EqualFold(scheme, "Bearer") && len(token) == 43
}

func writeSessionUnauthorized(c *gin.Context) {
	challenge := `Bearer realm="biznes"`
	if len(c.Request.Header.Values("Authorization")) != 0 {
		challenge += `, error="invalid_token"`
	}
	c.Header("WWW-Authenticate", challenge)
	WriteError(c, http.StatusUnauthorized, "unauthenticated", "Authentication is required or invalid.")
}

func currentUser(c *gin.Context) {
	user := c.MustGet("identity_user").(identity.User)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": userDTO(user)})
}

func logout(service *identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := service.Logout(c.Request.Context(), c.GetString("identity_token")); err != nil {
			writeIdentityError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNoContent)
	}
}

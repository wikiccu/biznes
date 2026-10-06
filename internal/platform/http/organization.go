package httpserver

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/identity"
	"github.com/wikiccu/biznes/internal/organization"
)

type publicOrganization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func organizationDTO(org organization.Organization) publicOrganization {
	return publicOrganization{org.ID, org.Name, org.Role, org.CreatedAt.UTC(), org.UpdatedAt.UTC()}
}

func createOrganization(service *organization.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var name string
		if !jsonStringInput(c, map[string]*string{"name": &name}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		org, err := service.Create(c.Request.Context(), user.ID, name)
		if err != nil {
			writeOrganizationError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+org.ID)
		c.JSON(http.StatusCreated, gin.H{"data": organizationDTO(org)})
	}
}

var canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func organizationID(c *gin.Context) (string, bool) {
	id := c.Param("organization_id")
	if !canonicalUUID.MatchString(id) {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide a canonical organization ID.",
			ErrorDetail{Field: "organization_id", Code: "invalid_format"})
		return "", false
	}
	return id, true
}

func getOrganization(service *organization.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := organizationID(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		org, err := service.Get(c.Request.Context(), user.ID, id)
		if err != nil {
			writeOrganizationError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": organizationDTO(org)})
	}
}

func renameOrganization(service *organization.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := organizationID(c)
		if !ok {
			return
		}
		var name string
		if !jsonStringInput(c, map[string]*string{"name": &name}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		org, err := service.Rename(c.Request.Context(), user.ID, id, name)
		if err != nil {
			writeOrganizationError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": organizationDTO(org)})
	}
}

func listOrganizations(service *organization.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, limit, ok := organizationPagination(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		orgs, err := service.List(c.Request.Context(), user.ID, page, limit)
		if err != nil {
			writeOrganizationError(c, err)
			return
		}
		data := make([]publicOrganization, 0, len(orgs))
		for _, org := range orgs {
			data = append(data, organizationDTO(org))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

func organizationPagination(c *gin.Context) (int, int, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 0)
	if _, err := io.ReadAll(c.Request.Body); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "This endpoint requires an empty request body.")
		return 0, 0, false
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || c.Request.URL.ForceQuery {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide valid pagination parameters.")
		return 0, 0, false
	}
	for key := range query {
		if key != "page" && key != "limit" {
			WriteError(c, http.StatusBadRequest, "invalid_request", "Only page and limit query parameters are accepted.")
			return 0, 0, false
		}
	}
	page, limit := 1, 20
	for _, field := range []struct {
		name  string
		value *int
	}{{"page", &page}, {"limit", &limit}} {
		values, present := query[field.name]
		if !present {
			continue
		}
		if len(values) != 1 || values[0] == "" {
			WriteError(c, http.StatusBadRequest, "invalid_request", "Provide one decimal value per pagination parameter.")
			return 0, 0, false
		}
		for _, char := range values[0] {
			if char < '0' || char > '9' {
				WriteError(c, http.StatusBadRequest, "invalid_request", "Pagination parameters require ASCII decimal digits.")
				return 0, 0, false
			}
		}
		value, err := strconv.Atoi(values[0])
		if err != nil {
			WriteError(c, http.StatusBadRequest, "invalid_request", "Pagination parameters are too large.")
			return 0, 0, false
		}
		*field.value = value
	}
	var details []ErrorDetail
	if page < 1 || page > 10000 {
		details = append(details, ErrorDetail{Field: "page", Code: "out_of_range"})
	}
	if limit < 1 || limit > 100 {
		details = append(details, ErrorDetail{Field: "limit", Code: "out_of_range"})
	}
	if len(details) != 0 {
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.", details...)
		return 0, 0, false
	}
	return page, limit, true
}

func writeOrganizationError(c *gin.Context, err error) {
	code := ""
	switch {
	case errors.Is(err, organization.ErrNameRequired):
		code = "required"
	case errors.Is(err, organization.ErrNameInvalid):
		code = "invalid_format"
	case errors.Is(err, organization.ErrNameTooLong):
		code = "out_of_range"
	case errors.Is(err, organization.ErrNotFound):
		WriteError(c, http.StatusNotFound, "not_found", "The requested resource was not found.")
	case errors.Is(err, organization.ErrForbidden):
		WriteError(c, http.StatusForbidden, "forbidden", "You do not have permission for this operation.")
	case errors.Is(err, organization.ErrUnavailable):
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "Organization service is temporarily unavailable.")
	default:
		WriteError(c, http.StatusInternalServerError, "internal_error", "An internal error occurred.")
	}
	if code != "" {
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.",
			ErrorDetail{Field: "name", Code: code})
	}
}

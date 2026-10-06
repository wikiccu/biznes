package httpserver

import (
	"errors"
	"net/http"
	"regexp"
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
	return resourceID(c, "organization_id")
}

func resourceID(c *gin.Context, field string) (string, bool) {
	id := c.Param(field)
	if !canonicalUUID.MatchString(id) {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide a canonical resource ID.",
			ErrorDetail{Field: field, Code: "invalid_format"})
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
		page, limit, ok := paginationInput(c)
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

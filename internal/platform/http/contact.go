package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/contact"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicContact struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func contactDTO(item contact.Contact) publicContact {
	return publicContact{item.ID, item.OrganizationID, item.Name, item.Kind, item.Email, item.Phone, item.Notes, item.CreatedAt.UTC(), item.UpdatedAt.UTC()}
}

func saveContact(service *contact.Service, replace bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var contactID string
		if replace {
			contactID, ok = resourceID(c, "contact_id")
			if !ok {
				return
			}
		}
		var input contact.Input
		if !jsonStringInput(c, map[string]*string{
			"name": &input.Name, "kind": &input.Kind, "email": &input.Email, "phone": &input.Phone, "notes": &input.Notes,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		var item contact.Contact
		var err error
		status := http.StatusCreated
		if replace {
			item, err = service.Replace(c.Request.Context(), user.ID, organizationID, contactID, input)
			status = http.StatusOK
		} else {
			item, err = service.Create(c.Request.Context(), user.ID, organizationID, input)
		}
		if err != nil {
			writeContactError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		if !replace {
			c.Header("Location", "/api/v1/organizations/"+organizationID+"/contacts/"+item.ID)
		}
		c.JSON(status, gin.H{"data": contactDTO(item)})
	}
}

func getContact(service *contact.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "contact_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.Get(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeContactError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": contactDTO(item)})
	}
}

func listContacts(service *contact.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		page, limit, ok := paginationInput(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		items, err := service.List(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeContactError(c, err)
			return
		}
		data := make([]publicContact, 0, len(items))
		for _, item := range items {
			data = append(data, contactDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

func writeContactError(c *gin.Context, err error) {
	var validation *contact.ValidationError
	switch {
	case errors.As(err, &validation):
		details := make([]ErrorDetail, 0, len(validation.Fields))
		for _, field := range validation.Fields {
			details = append(details, ErrorDetail{Field: field.Field, Code: field.Code})
		}
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.", details...)
	case errors.Is(err, contact.ErrUnavailable):
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "Contact service is temporarily unavailable.")
	default:
		writeOrganizationError(c, err)
	}
}

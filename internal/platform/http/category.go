package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicCategory struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func categoryDTO(item finance.Category) publicCategory {
	return publicCategory{item.ID, item.OrganizationID, item.Name, item.Kind, item.CreatedAt.UTC(), item.UpdatedAt.UTC()}
}

func saveCategory(service *finance.Service, rename bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var categoryID string
		var input finance.CategoryInput
		fields := map[string]*string{"name": &input.Name}
		if rename {
			categoryID, ok = resourceID(c, "category_id")
			if !ok {
				return
			}
		} else {
			fields["kind"] = &input.Kind
		}
		if !jsonStringInput(c, fields) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		var item finance.Category
		var err error
		status := http.StatusCreated
		if rename {
			item, err = service.RenameCategory(c.Request.Context(), user.ID, organizationID, categoryID, input.Name)
			status = http.StatusOK
		} else {
			item, err = service.CreateCategory(c.Request.Context(), user.ID, organizationID, input)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		if !rename {
			c.Header("Location", "/api/v1/organizations/"+organizationID+"/transaction-categories/"+item.ID)
		}
		c.JSON(status, gin.H{"data": categoryDTO(item)})
	}
}

func getCategory(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "category_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetCategory(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": categoryDTO(item)})
	}
}

func listCategories(service *finance.Service) gin.HandlerFunc {
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
		items, err := service.ListCategories(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicCategory, 0, len(items))
		for _, item := range items {
			data = append(data, categoryDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

func writeFinanceError(c *gin.Context, err error) {
	var validation *finance.ValidationError
	switch {
	case errors.As(err, &validation):
		details := make([]ErrorDetail, 0, len(validation.Fields))
		for _, field := range validation.Fields {
			details = append(details, ErrorDetail{Field: field.Field, Code: field.Code})
		}
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.", details...)
	case errors.Is(err, finance.ErrCategoryConflict):
		WriteError(c, http.StatusConflict, "conflict", "A category with this name and kind already exists.")
	case errors.Is(err, finance.ErrAccountConflict):
		WriteError(c, http.StatusConflict, "conflict", "An account with this name already exists.")
	case errors.Is(err, finance.ErrTransactionConflict):
		WriteError(c, http.StatusConflict, "conflict", "This idempotency key was already used with different transaction input.")
	case errors.Is(err, finance.ErrReversalConflict):
		WriteError(c, http.StatusConflict, "conflict", "This transaction is already reversed or the key was used with different reversal input.")
	case errors.Is(err, finance.ErrReceivableConflict):
		WriteError(c, http.StatusConflict, "conflict", "This idempotency key was already used with different receivable input.")
	case errors.Is(err, finance.ErrPayableConflict):
		WriteError(c, http.StatusConflict, "conflict", "This idempotency key was already used with different payable input.")
	case errors.Is(err, finance.ErrAllocationConflict):
		WriteError(c, http.StatusConflict, "conflict", "This allocation exceeds available amounts, uses an ineligible receipt, or reuses a key with different input.")
	case errors.Is(err, finance.ErrPayableAllocationConflict):
		WriteError(c, http.StatusConflict, "conflict", "This allocation exceeds available amounts, uses an ineligible payment, or reuses a key with different input.")
	case errors.Is(err, finance.ErrUnavailable):
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "Finance service is temporarily unavailable.")
	default:
		writeOrganizationError(c, err)
	}
}

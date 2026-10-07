package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicReceivable struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	ContactID      string    `json:"contact_id"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	DueDate        string    `json:"due_date"`
	Description    string    `json:"description"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func receivableDTO(item finance.Receivable) publicReceivable {
	return publicReceivable{item.ID, item.OrganizationID, item.IdempotencyKey, item.ContactID,
		strconv.FormatInt(item.Amount, 10), item.Currency, item.DueDate.Format(time.DateOnly), item.Description, item.CreatedBy, item.CreatedAt.UTC()}
}

func recordReceivable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var input finance.ReceivableInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "contact_id": &input.ContactID, "amount": &input.Amount,
			"currency": &input.Currency, "due_date": &input.DueDate, "description": &input.Description,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.RecordReceivable(c.Request.Context(), user.ID, organizationID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/receivables/"+item.ID)
		c.JSON(status, gin.H{"data": receivableDTO(item)})
	}
}

func getReceivable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "receivable_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetReceivable(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": receivableDTO(item)})
	}
}

func listReceivables(service *finance.Service) gin.HandlerFunc {
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
		items, err := service.ListReceivables(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicReceivable, 0, len(items))
		for _, item := range items {
			data = append(data, receivableDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

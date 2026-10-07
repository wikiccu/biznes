package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

func recordPayable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var input finance.PayableInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "contact_id": &input.ContactID, "amount": &input.Amount,
			"currency": &input.Currency, "due_date": &input.DueDate, "description": &input.Description,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.RecordPayable(c.Request.Context(), user.ID, organizationID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/payables/"+item.ID)
		c.JSON(status, gin.H{"data": debtDTO(item)})
	}
}

func getPayable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "payable_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayable(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": debtDTO(item)})
	}
}

func listPayables(service *finance.Service) gin.HandlerFunc {
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
		items, err := service.ListPayables(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicDebt, 0, len(items))
		for _, item := range items {
			data = append(data, debtDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

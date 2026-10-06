package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicReversal struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	TransactionID  string    `json:"transaction_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Reason         string    `json:"reason"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func reversalDTO(item finance.Reversal) publicReversal {
	return publicReversal{item.ID, item.OrganizationID, item.TransactionID, item.IdempotencyKey, item.Reason, item.CreatedBy, item.CreatedAt.UTC()}
}

func reverseTransaction(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		transactionID, ok := resourceID(c, "transaction_id")
		if !ok {
			return
		}
		var input finance.ReversalInput
		if !jsonStringInput(c, map[string]*string{"idempotency_key": &input.IdempotencyKey, "reason": &input.Reason}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.ReverseTransaction(c.Request.Context(), user.ID, organizationID, transactionID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/transactions/"+transactionID+"/reversal")
		c.JSON(status, gin.H{"data": reversalDTO(item)})
	}
}

func getReversal(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		transactionID, ok := resourceID(c, "transaction_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetReversal(c.Request.Context(), user.ID, organizationID, transactionID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": reversalDTO(item)})
	}
}

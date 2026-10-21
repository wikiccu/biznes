package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicObligationCancellation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	ObligationID   string    `json:"obligation_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Reason         string    `json:"reason"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func obligationCancellationDTO(item finance.ObligationCancellation) publicObligationCancellation {
	return publicObligationCancellation{item.ID, item.OrganizationID, item.ObligationID, item.IdempotencyKey,
		item.Reason, item.CreatedBy, item.CreatedAt.UTC()}
}

func cancelObligation(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		obligationID, ok := resourceID(c, "obligation_id")
		if !ok {
			return
		}
		var input finance.ObligationCancellationInput
		if !jsonStringInput(c, map[string]*string{"idempotency_key": &input.IdempotencyKey, "reason": &input.Reason}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.CancelObligation(c.Request.Context(), user.ID, organizationID, obligationID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/financial-obligations/"+obligationID+"/cancellation")
		c.JSON(status, gin.H{"data": obligationCancellationDTO(item)})
	}
}

func getObligationCancellation(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		obligationID, ok := resourceID(c, "obligation_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetObligationCancellation(c.Request.Context(), user.ID, organizationID, obligationID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": obligationCancellationDTO(item)})
	}
}

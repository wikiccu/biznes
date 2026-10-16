package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicInstrumentCancellation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	InstrumentID   string    `json:"instrument_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Reason         string    `json:"reason"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func instrumentCancellationDTO(item finance.InstrumentCancellation) publicInstrumentCancellation {
	return publicInstrumentCancellation{item.ID, item.OrganizationID, item.InstrumentID, item.IdempotencyKey, item.Reason, item.CreatedBy, item.CreatedAt.UTC()}
}

func cancelPayableInstrument(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		instrumentID, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		var input finance.InstrumentCancellationInput
		if !jsonStringInput(c, map[string]*string{"idempotency_key": &input.IdempotencyKey, "reason": &input.Reason}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.CancelPayableInstrument(c.Request.Context(), user.ID, organizationID, instrumentID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/payable-instruments/"+instrumentID+"/cancellation")
		c.JSON(status, gin.H{"data": instrumentCancellationDTO(item)})
	}
}

func getPayableInstrumentCancellation(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		instrumentID, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayableInstrumentCancellation(c.Request.Context(), user.ID, organizationID, instrumentID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": instrumentCancellationDTO(item)})
	}
}

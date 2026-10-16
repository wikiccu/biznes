package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicInstrumentSettlement struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	InstrumentID   string    `json:"instrument_id"`
	AllocationID   string    `json:"allocation_id"`
	PayableID      string    `json:"payable_id"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func instrumentSettlementDTO(item finance.InstrumentSettlement) publicInstrumentSettlement {
	return publicInstrumentSettlement{item.ID, item.OrganizationID, item.IdempotencyKey, item.InstrumentID,
		item.AllocationID, item.PayableID, strconv.FormatInt(item.Amount, 10), item.Currency, item.CreatedBy, item.CreatedAt.UTC()}
}

func settlePayableInstrument(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		instrumentID, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		var input finance.InstrumentSettlementInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "allocation_id": &input.AllocationID, "amount": &input.Amount,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.SettlePayableInstrument(c.Request.Context(), user.ID, organizationID, instrumentID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/payable-instruments/"+instrumentID+"/settlements/"+item.ID)
		c.JSON(status, gin.H{"data": instrumentSettlementDTO(item)})
	}
}

func getPayableInstrumentSettlement(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		instrumentID, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		settlementID, ok := resourceID(c, "settlement_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayableInstrumentSettlement(c.Request.Context(), user.ID, organizationID, instrumentID, settlementID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": instrumentSettlementDTO(item)})
	}
}

func listPayableInstrumentSettlements(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		instrumentID, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		page, limit, ok := paginationInput(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		items, err := service.ListPayableInstrumentSettlements(c.Request.Context(), user.ID, organizationID, instrumentID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicInstrumentSettlement, 0, len(items))
		for _, item := range items {
			data = append(data, instrumentSettlementDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

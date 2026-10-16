package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicPayableInstrument struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	PayableID      string    `json:"payable_id"`
	Kind           string    `json:"kind"`
	Direction      string    `json:"direction"`
	Status         string    `json:"status"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	DueDate        string    `json:"due_date"`
	BankName       string    `json:"bank_name"`
	Reference      string    `json:"reference"`
	Description    string    `json:"description"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func payableInstrumentDTO(item finance.PayableInstrument) publicPayableInstrument {
	return publicPayableInstrument{item.ID, item.OrganizationID, item.IdempotencyKey, item.PayableID,
		item.Kind, "outgoing", "recorded", strconv.FormatInt(item.Amount, 10), item.Currency,
		item.DueDate.Format(time.DateOnly), item.BankName, item.Reference, item.Description, item.CreatedBy, item.CreatedAt.UTC()}
}

func recordPayableInstrument(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var input finance.PayableInstrumentInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "payable_id": &input.PayableID, "kind": &input.Kind,
			"amount": &input.Amount, "currency": &input.Currency, "due_date": &input.DueDate,
			"bank_name": &input.BankName, "reference": &input.Reference, "description": &input.Description,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.RecordPayableInstrument(c.Request.Context(), user.ID, organizationID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/payable-instruments/"+item.ID)
		c.JSON(status, gin.H{"data": payableInstrumentDTO(item)})
	}
}

func getPayableInstrument(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "instrument_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayableInstrument(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": payableInstrumentDTO(item)})
	}
}

func listPayableInstruments(service *finance.Service) gin.HandlerFunc {
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
		items, err := service.ListPayableInstruments(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicPayableInstrument, 0, len(items))
		for _, item := range items {
			data = append(data, payableInstrumentDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

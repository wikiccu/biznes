package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

func instrumentDTO(item finance.Instrument, payable bool) gin.H {
	debtField, direction := "receivable_id", "incoming"
	if payable {
		debtField, direction = "payable_id", "outgoing"
	}
	return gin.H{
		"id": item.ID, "organization_id": item.OrganizationID, "idempotency_key": item.IdempotencyKey,
		debtField: item.DebtID, "kind": item.Kind, "direction": direction, "status": item.Status,
		"amount": strconv.FormatInt(item.Amount, 10), "currency": item.Currency,
		"due_date": item.DueDate.Format(time.DateOnly), "bank_name": item.BankName, "reference": item.Reference,
		"description": item.Description, "created_by": item.CreatedBy, "created_at": item.CreatedAt.UTC(),
	}
}

func recordInstrument(service *finance.Service, payable bool) gin.HandlerFunc {
	debtField, route := "receivable_id", "receivable-instruments"
	if payable {
		debtField, route = "payable_id", "payable-instruments"
	}
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var input finance.InstrumentInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, debtField: &input.DebtID, "kind": &input.Kind,
			"amount": &input.Amount, "currency": &input.Currency, "due_date": &input.DueDate,
			"bank_name": &input.BankName, "reference": &input.Reference, "description": &input.Description,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		var item finance.Instrument
		var created bool
		var err error
		if payable {
			item, created, err = service.RecordPayableInstrument(c.Request.Context(), user.ID, organizationID, input)
		} else {
			item, created, err = service.RecordReceivableInstrument(c.Request.Context(), user.ID, organizationID, input)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/"+route+"/"+item.ID)
		c.JSON(status, gin.H{"data": instrumentDTO(item, payable)})
	}
}

func getInstrument(service *finance.Service, payable bool) gin.HandlerFunc {
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
		var item finance.Instrument
		var err error
		if payable {
			item, err = service.GetPayableInstrument(c.Request.Context(), user.ID, organizationID, id)
		} else {
			item, err = service.GetReceivableInstrument(c.Request.Context(), user.ID, organizationID, id)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": instrumentDTO(item, payable)})
	}
}

func listInstruments(service *finance.Service, payable bool) gin.HandlerFunc {
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
		var items []finance.Instrument
		var err error
		if payable {
			items, err = service.ListPayableInstruments(c.Request.Context(), user.ID, organizationID, page, limit)
		} else {
			items, err = service.ListReceivableInstruments(c.Request.Context(), user.ID, organizationID, page, limit)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]gin.H, 0, len(items))
		for _, item := range items {
			data = append(data, instrumentDTO(item, payable))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

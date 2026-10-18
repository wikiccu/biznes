package httpserver

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

func instrumentSettlementDTO(item finance.InstrumentSettlement, payable bool) gin.H {
	debtField := "receivable_id"
	if payable {
		debtField = "payable_id"
	}
	return gin.H{
		"id": item.ID, "organization_id": item.OrganizationID, "idempotency_key": item.IdempotencyKey,
		"instrument_id": item.InstrumentID, "allocation_id": item.AllocationID, debtField: item.DebtID,
		"amount": strconv.FormatInt(item.Amount, 10), "currency": item.Currency,
		"created_by": item.CreatedBy, "created_at": item.CreatedAt.UTC(),
	}
}

func settleInstrument(service *finance.Service, payable bool) gin.HandlerFunc {
	route := "receivable-instruments"
	if payable {
		route = "payable-instruments"
	}
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
		var item finance.InstrumentSettlement
		var created bool
		var err error
		if payable {
			item, created, err = service.SettlePayableInstrument(c.Request.Context(), user.ID, organizationID, instrumentID, input)
		} else {
			item, created, err = service.SettleReceivableInstrument(c.Request.Context(), user.ID, organizationID, instrumentID, input)
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
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/"+route+"/"+instrumentID+"/settlements/"+item.ID)
		c.JSON(status, gin.H{"data": instrumentSettlementDTO(item, payable)})
	}
}

func getInstrumentSettlement(service *finance.Service, payable bool) gin.HandlerFunc {
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
		var item finance.InstrumentSettlement
		var err error
		if payable {
			item, err = service.GetPayableInstrumentSettlement(c.Request.Context(), user.ID, organizationID, instrumentID, settlementID)
		} else {
			item, err = service.GetReceivableInstrumentSettlement(c.Request.Context(), user.ID, organizationID, instrumentID, settlementID)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": instrumentSettlementDTO(item, payable)})
	}
}

func listInstrumentSettlements(service *finance.Service, payable bool) gin.HandlerFunc {
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
		var items []finance.InstrumentSettlement
		var err error
		if payable {
			items, err = service.ListPayableInstrumentSettlements(c.Request.Context(), user.ID, organizationID, instrumentID, page, limit)
		} else {
			items, err = service.ListReceivableInstrumentSettlements(c.Request.Context(), user.ID, organizationID, instrumentID, page, limit)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]gin.H, 0, len(items))
		for _, item := range items {
			data = append(data, instrumentSettlementDTO(item, payable))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

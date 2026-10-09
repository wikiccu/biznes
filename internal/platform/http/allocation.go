package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicReceivableAllocation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	ReceivableID   string    `json:"receivable_id"`
	TransactionID  string    `json:"transaction_id"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func receivableAllocationDTO(item finance.ReceivableAllocation) publicReceivableAllocation {
	return publicReceivableAllocation{item.ID, item.OrganizationID, item.IdempotencyKey, item.ReceivableID,
		item.TransactionID, strconv.FormatInt(item.Amount, 10), item.Currency, item.CreatedBy, item.CreatedAt.UTC()}
}

func allocateReceivable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		receivableID, ok := resourceID(c, "receivable_id")
		if !ok {
			return
		}
		var input finance.ReceivableAllocationInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "transaction_id": &input.TransactionID, "amount": &input.Amount,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.AllocateReceivable(c.Request.Context(), user.ID, organizationID, receivableID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/receivables/"+receivableID+"/allocations/"+item.ID)
		c.JSON(status, gin.H{"data": receivableAllocationDTO(item)})
	}
}

func getReceivableAllocation(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		receivableID, ok := resourceID(c, "receivable_id")
		if !ok {
			return
		}
		allocationID, ok := resourceID(c, "allocation_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetReceivableAllocation(c.Request.Context(), user.ID, organizationID, receivableID, allocationID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": receivableAllocationDTO(item)})
	}
}

func listReceivableAllocations(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		receivableID, ok := resourceID(c, "receivable_id")
		if !ok {
			return
		}
		page, limit, ok := paginationInput(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		items, err := service.ListReceivableAllocations(c.Request.Context(), user.ID, organizationID, receivableID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicReceivableAllocation, 0, len(items))
		for _, item := range items {
			data = append(data, receivableAllocationDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

func getReceivableCollectionSummary(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		receivableID, ok := resourceID(c, "receivable_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetReceivableCollectionSummary(c.Request.Context(), user.ID, organizationID, receivableID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"organization_id": item.OrganizationID, "receivable_id": item.ReceivableID, "currency": item.Currency,
			"amount": item.Amount, "collected_amount": item.CollectedAmount, "outstanding_amount": item.OutstandingAmount,
			"basis": "recorded_allocations",
		}})
	}
}

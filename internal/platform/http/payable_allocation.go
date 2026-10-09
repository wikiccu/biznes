package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicPayableAllocation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	PayableID      string    `json:"payable_id"`
	TransactionID  string    `json:"transaction_id"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func payableAllocationDTO(item finance.Allocation) publicPayableAllocation {
	return publicPayableAllocation{item.ID, item.OrganizationID, item.IdempotencyKey, item.DebtID,
		item.TransactionID, strconv.FormatInt(item.Amount, 10), item.Currency, item.CreatedBy, item.CreatedAt.UTC()}
}

func allocatePayable(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		payableID, ok := resourceID(c, "payable_id")
		if !ok {
			return
		}
		var input finance.AllocationInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "transaction_id": &input.TransactionID, "amount": &input.Amount,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.AllocatePayable(c.Request.Context(), user.ID, organizationID, payableID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/payables/"+payableID+"/allocations/"+item.ID)
		c.JSON(status, gin.H{"data": payableAllocationDTO(item)})
	}
}

func getPayableAllocation(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		payableID, ok := resourceID(c, "payable_id")
		if !ok {
			return
		}
		allocationID, ok := resourceID(c, "allocation_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayableAllocation(c.Request.Context(), user.ID, organizationID, payableID, allocationID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": payableAllocationDTO(item)})
	}
}

func listPayableAllocations(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		payableID, ok := resourceID(c, "payable_id")
		if !ok {
			return
		}
		page, limit, ok := paginationInput(c)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		items, err := service.ListPayableAllocations(c.Request.Context(), user.ID, organizationID, payableID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicPayableAllocation, 0, len(items))
		for _, item := range items {
			data = append(data, payableAllocationDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

func getPayablePaymentSummary(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		payableID, ok := resourceID(c, "payable_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetPayablePaymentSummary(c.Request.Context(), user.ID, organizationID, payableID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"organization_id": item.OrganizationID, "payable_id": item.DebtID, "currency": item.Currency,
			"amount": item.Amount, "paid_amount": item.AllocatedAmount, "outstanding_amount": item.OutstandingAmount,
			"basis": "recorded_allocations",
		}})
	}
}

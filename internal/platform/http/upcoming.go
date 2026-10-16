package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

func listUpcomingDebts(service *finance.Service, payable bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		query, ok := queryInput(c, "page", "limit", "due_from", "due_to")
		if !ok {
			return
		}
		page, limit, ok := paginationValues(c, query)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		input := finance.DueDateRange{From: query.Get("due_from"), To: query.Get("due_to")}
		var items []finance.UpcomingDebt
		var err error
		idField, allocatedField := "receivable_id", "collected_amount"
		if payable {
			items, err = service.ListUpcomingPayables(c.Request.Context(), user.ID, organizationID, page, limit, input)
			idField, allocatedField = "payable_id", "paid_amount"
		} else {
			items, err = service.ListUpcomingReceivables(c.Request.Context(), user.ID, organizationID, page, limit, input)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]gin.H, 0, len(items))
		for _, item := range items {
			data = append(data, gin.H{
				idField: item.ID, "contact_id": item.ContactID, "amount": item.Amount, "currency": item.Currency,
				"due_date": item.DueDate.Format(time.DateOnly), allocatedField: item.AllocatedAmount,
				"outstanding_amount": item.OutstandingAmount,
			})
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{
			"organization_id": organizationID, "page": page, "limit": limit,
			"due_from": query.Get("due_from"), "due_to": query.Get("due_to"), "basis": "current_recorded_allocations",
		}})
	}
}

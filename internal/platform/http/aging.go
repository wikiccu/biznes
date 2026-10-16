package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

func getDebtAging(service *finance.Service, payable bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		query, ok := queryInput(c, "as_of")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		var item finance.DebtAging
		var err error
		if payable {
			item, err = service.GetPayableAging(c.Request.Context(), user.ID, organizationID, query.Get("as_of"))
		} else {
			item, err = service.GetReceivableAging(c.Request.Context(), user.ID, organizationID, query.Get("as_of"))
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		buckets := make([]gin.H, 0, len(item.Buckets))
		for _, bucket := range item.Buckets {
			buckets = append(buckets, gin.H{"bucket": bucket.Bucket, "outstanding_amount": bucket.OutstandingAmount})
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"organization_id": item.OrganizationID, "currency": "IRR", "as_of": item.AsOf.Format(time.DateOnly),
			"basis": "current_recorded_allocations", "outstanding_amount": item.OutstandingAmount,
			"overdue_amount": item.OverdueAmount, "buckets": buckets,
		}})
	}
}

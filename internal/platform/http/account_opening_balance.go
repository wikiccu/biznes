package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicAccountOpeningBalance struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AccountID      string    `json:"account_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	EffectiveAt    time.Time `json:"effective_at"`
	Reason         string    `json:"reason"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func accountOpeningBalanceDTO(item finance.AccountOpeningBalance) publicAccountOpeningBalance {
	return publicAccountOpeningBalance{item.ID, item.OrganizationID, item.AccountID, item.IdempotencyKey,
		strconv.FormatInt(item.Amount, 10), item.Currency, item.EffectiveAt.UTC(), item.Reason, item.CreatedBy, item.CreatedAt.UTC()}
}

func recordAccountOpeningBalance(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		accountID, ok := resourceID(c, "account_id")
		if !ok {
			return
		}
		var input finance.AccountOpeningBalanceInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "amount": &input.Amount, "currency": &input.Currency,
			"effective_at": &input.EffectiveAt, "reason": &input.Reason,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.RecordAccountOpeningBalance(c.Request.Context(), user.ID, organizationID, accountID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/financial-accounts/"+accountID+"/opening-balance")
		c.JSON(status, gin.H{"data": accountOpeningBalanceDTO(item)})
	}
}

func getAccountOpeningBalance(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		accountID, ok := resourceID(c, "account_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetAccountOpeningBalance(c.Request.Context(), user.ID, organizationID, accountID)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": accountOpeningBalanceDTO(item)})
	}
}

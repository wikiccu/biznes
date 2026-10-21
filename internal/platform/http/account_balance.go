package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicAccountBalance struct {
	OrganizationID   string    `json:"organization_id"`
	AccountID        string    `json:"account_id"`
	Currency         string    `json:"currency"`
	OpeningBalanceID string    `json:"opening_balance_id"`
	OpeningAmount    string    `json:"opening_amount"`
	EffectiveAt      time.Time `json:"effective_at"`
	AsOf             time.Time `json:"as_of"`
	IncomeAmount     string    `json:"income_amount"`
	ExpenseAmount    string    `json:"expense_amount"`
	NetAmount        string    `json:"net_amount"`
	BalanceAmount    string    `json:"balance_amount"`
	Basis            string    `json:"basis"`
}

func getAccountBalance(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		accountID, ok := resourceID(c, "account_id")
		if !ok {
			return
		}
		query, ok := queryInput(c, "as_of")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetAccountBalance(c.Request.Context(), user.ID, organizationID, accountID, query.Get("as_of"))
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": publicAccountBalance{
			OrganizationID: item.OrganizationID, AccountID: item.AccountID, Currency: item.Currency,
			OpeningBalanceID: item.OpeningBalanceID, OpeningAmount: item.OpeningAmount,
			EffectiveAt: item.EffectiveAt.UTC(), AsOf: item.AsOf.UTC(),
			IncomeAmount: item.IncomeAmount, ExpenseAmount: item.ExpenseAmount,
			NetAmount: item.NetAmount, BalanceAmount: item.BalanceAmount, Basis: "opening_and_recorded_transactions",
		}})
	}
}

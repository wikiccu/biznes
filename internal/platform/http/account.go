package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicAccount struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type publicAccountActivity struct {
	OrganizationID         string     `json:"organization_id"`
	AccountID              string     `json:"account_id"`
	Currency               string     `json:"currency"`
	IncomeAmount           string     `json:"income_amount"`
	ExpenseAmount          string     `json:"expense_amount"`
	NetAmount              string     `json:"net_amount"`
	Basis                  string     `json:"basis"`
	OpeningBalanceIncluded bool       `json:"opening_balance_included"`
	From                   *time.Time `json:"from,omitempty"`
	To                     *time.Time `json:"to,omitempty"`
}

func accountDTO(item finance.Account) publicAccount {
	return publicAccount{item.ID, item.OrganizationID, item.Name, item.Kind, item.Currency, item.CreatedAt.UTC(), item.UpdatedAt.UTC()}
}

func saveAccount(service *finance.Service, rename bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var accountID string
		var input finance.AccountInput
		fields := map[string]*string{"name": &input.Name}
		if rename {
			accountID, ok = resourceID(c, "account_id")
			if !ok {
				return
			}
		} else {
			fields["kind"] = &input.Kind
			fields["currency"] = &input.Currency
		}
		if !jsonStringInput(c, fields) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		var item finance.Account
		var err error
		status := http.StatusCreated
		if rename {
			item, err = service.RenameAccount(c.Request.Context(), user.ID, organizationID, accountID, input.Name)
			status = http.StatusOK
		} else {
			item, err = service.CreateAccount(c.Request.Context(), user.ID, organizationID, input)
		}
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		if !rename {
			c.Header("Location", "/api/v1/organizations/"+organizationID+"/financial-accounts/"+item.ID)
		}
		c.JSON(status, gin.H{"data": accountDTO(item)})
	}
}

func getAccount(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "account_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetAccount(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": accountDTO(item)})
	}
}

func getAccountActivity(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "account_id")
		if !ok {
			return
		}
		query, ok := queryInput(c, "from", "to")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetAccountActivity(c.Request.Context(), user.ID, organizationID,
			finance.TransactionFilter{AccountID: id, From: query.Get("from"), To: query.Get("to")})
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": publicAccountActivity{
			OrganizationID: item.OrganizationID, AccountID: item.AccountID, Currency: item.Currency,
			IncomeAmount: item.IncomeAmount, ExpenseAmount: item.ExpenseAmount, NetAmount: item.NetAmount,
			Basis: "recorded_transactions", OpeningBalanceIncluded: false,
			From: item.From, To: item.To,
		}})
	}
}

func listAccounts(service *finance.Service) gin.HandlerFunc {
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
		items, err := service.ListAccounts(c.Request.Context(), user.ID, organizationID, page, limit)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicAccount, 0, len(items))
		for _, item := range items {
			data = append(data, accountDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

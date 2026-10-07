package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
)

type publicTransaction struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	AccountID      string    `json:"account_id"`
	CategoryID     string    `json:"category_id"`
	Kind           string    `json:"kind"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	OccurredAt     time.Time `json:"occurred_at"`
	Description    string    `json:"description"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func transactionDTO(item finance.Transaction) publicTransaction {
	return publicTransaction{item.ID, item.OrganizationID, item.IdempotencyKey, item.AccountID, item.CategoryID, item.Kind,
		strconv.FormatInt(item.Amount, 10), item.Currency, item.OccurredAt.UTC(), item.Description, item.CreatedBy, item.CreatedAt.UTC()}
}

func recordTransaction(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		var input finance.TransactionInput
		if !jsonStringInput(c, map[string]*string{
			"idempotency_key": &input.IdempotencyKey, "account_id": &input.AccountID, "category_id": &input.CategoryID,
			"amount": &input.Amount, "currency": &input.Currency, "occurred_at": &input.OccurredAt, "description": &input.Description,
		}) {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, created, err := service.RecordTransaction(c.Request.Context(), user.ID, organizationID, input)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Location", "/api/v1/organizations/"+organizationID+"/transactions/"+item.ID)
		c.JSON(status, gin.H{"data": transactionDTO(item)})
	}
}

func getTransaction(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		id, ok := resourceID(c, "transaction_id")
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		item, err := service.GetTransaction(c.Request.Context(), user.ID, organizationID, id)
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": transactionDTO(item)})
	}
}

func listTransactions(service *finance.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, ok := organizationID(c)
		if !ok {
			return
		}
		query, ok := queryInput(c, "page", "limit", "account_id", "from", "to")
		if !ok {
			return
		}
		page, limit, ok := paginationValues(c, query)
		if !ok {
			return
		}
		user := c.MustGet("identity_user").(identity.User)
		items, err := service.ListTransactions(c.Request.Context(), user.ID, organizationID, page, limit,
			finance.TransactionFilter{AccountID: query.Get("account_id"), From: query.Get("from"), To: query.Get("to")})
		if err != nil {
			writeFinanceError(c, err)
			return
		}
		data := make([]publicTransaction, 0, len(items))
		for _, item := range items {
			data = append(data, transactionDTO(item))
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit}})
	}
}

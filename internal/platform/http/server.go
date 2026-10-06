package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wikiccu/biznes/internal/contact"
	"github.com/wikiccu/biznes/internal/finance"
	"github.com/wikiccu/biznes/internal/identity"
	"github.com/wikiccu/biznes/internal/organization"
	"github.com/wikiccu/biznes/internal/platform/config"
)

func New(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.HandleMethodNotAllowed = true
	_ = router.SetTrustedProxies(nil)
	router.Use(requestID(), requestLogging(logger), recovery(logger))
	router.NoRoute(func(c *gin.Context) {
		WriteError(c, http.StatusNotFound, "not_found", "The requested resource was not found.")
	})
	router.NoMethod(func(c *gin.Context) {
		WriteError(c, http.StatusMethodNotAllowed, "method_not_allowed", "The request method is not supported for this resource.")
	})
	identityService := identity.NewService(pool)
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", registration(identityService))
	auth.POST("/login", login(identityService))
	protected := auth.Group("", emptyAuthRequest, sessionAuthentication(identityService))
	protected.GET("/me", currentUser)
	protected.POST("/logout", logout(identityService))
	organizationService := organization.NewService(pool)
	organizations := router.Group("/api/v1/organizations", sessionAuthentication(identityService))
	organizations.POST("", createOrganization(organizationService))
	organizations.GET("", listOrganizations(organizationService))
	organizations.GET("/:organization_id", emptyAuthRequest, getOrganization(organizationService))
	organizations.PATCH("/:organization_id", renameOrganization(organizationService))
	contactService := contact.NewService(pool)
	contacts := organizations.Group("/:organization_id/contacts")
	contacts.POST("", saveContact(contactService, false))
	contacts.GET("", listContacts(contactService))
	contacts.GET("/:contact_id", emptyAuthRequest, getContact(contactService))
	contacts.PUT("/:contact_id", saveContact(contactService, true))
	financeService := finance.NewService(pool)
	categories := organizations.Group("/:organization_id/transaction-categories")
	categories.POST("", saveCategory(financeService, false))
	categories.GET("", listCategories(financeService))
	categories.GET("/:category_id", emptyAuthRequest, getCategory(financeService))
	categories.PATCH("/:category_id", saveCategory(financeService, true))
	accounts := organizations.Group("/:organization_id/financial-accounts")
	accounts.POST("", saveAccount(financeService, false))
	accounts.GET("", listAccounts(financeService))
	accounts.GET("/:account_id", emptyAuthRequest, getAccount(financeService))
	accounts.PATCH("/:account_id", saveAccount(financeService, true))
	accounts.GET("/:account_id/recorded-activity", emptyAuthRequest, getAccountActivity(financeService))
	transactions := organizations.Group("/:organization_id/transactions")
	transactions.POST("", recordTransaction(financeService))
	transactions.GET("", listTransactions(financeService))
	transactions.GET("/:transaction_id", emptyAuthRequest, getTransaction(financeService))
	transactions.POST("/:transaction_id/reversal", reverseTransaction(financeService))
	transactions.GET("/:transaction_id/reversal", emptyAuthRequest, getReversal(financeService))

	router.GET("/health", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ready", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		// Observe shutdown without canceling in-flight request contexts.
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), cfg.DatabaseHealthTimeout)
		defer cancel()
		stopCancel := context.AfterFunc(ctx, cancel)
		defer stopCancel()
		if ctx.Err() != nil || pool.Ping(checkCtx) != nil || ctx.Err() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	return &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

func Run(ctx context.Context, server *http.Server, shutdownTimeout time.Duration, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	logger.Info("HTTP server listening", "address", listener.Addr().String())

	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.Join(fmt.Errorf("serve HTTP: %w", err), server.Close())
	case <-ctx.Done():
	}

	logger.Info("HTTP server stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Join(fmt.Errorf("shutdown HTTP: %w", err), server.Close())
	}
	if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	logger.Info("HTTP server stopped")
	return nil
}

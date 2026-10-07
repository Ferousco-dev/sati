package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"
	"github.com/Ferousco-dev/sati/backend/pkg/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ferousco-dev/sati/backend/api"
	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/audit"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/cms"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/notifications"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/server"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	limiter, redisClient := newLimiter(ctx, cfg, log)
	if redisClient != nil {
		defer redisClient.Close()
	}
	registerGauges(pool)

	notifier := notifications.NewService(cfg.FrontendURL, log)
	mailer := notifications.NewMailer(notifier, pool)
	recorder := audit.NewRecorder(pool, log)
	authSvc := auth.NewService(pool, mailer, log, cfg.SessionSecret, auth.DefaultSettings())

	storage, err := files.NewLocalStorage(cfg.UploadDir, cfg.AssetBaseURL)
	if cfg.StorageDriver != "local" {
		err = fmt.Errorf("storage driver %q is not implemented yet", cfg.StorageDriver)
	}
	if err != nil {
		return err
	}

	storeLocation, err := time.LoadLocation(cfg.StoreTimezone)
	if err != nil {
		return err
	}

	orderSvc := orders.NewService(pool, notifier, log)
	var paymentSvc payments.Service
	if cfg.PaystackSecretKey != "" {
		orderSvc.EnableOnline(payments.NewPaystack(cfg.PaystackSecretKey, cfg.PaystackWebhookSecret), cfg.FrontendURL)
		paymentSvc = orderSvc
	} else {
		log.Warn("PAYSTACK_SECRET_KEY is not set, online payment is switched off")
	}

	categorySvc := categories.NewService(categories.NewRepository(pool))
	productSvc := products.NewService(pool, storage, log)
	paymentsHandler := payments.NewHandler(pool, log)
	if paymentSvc != nil {
		paymentsHandler.WithService(paymentSvc)
	}
	healthHandler := health.New(pool)
	if redisClient != nil {
		healthHandler.WithRedis(redisPinger{redisClient})
	}
	handlers := server.Handlers{
		OpenAPI:       api.Spec,
		Limiter:       limiter,
		MetricsToken:  cfg.MetricsToken,
		Health:        healthHandler,
		Auth:          auth.NewHandler(authSvc, log, cfg.IsProduction()).WithAuditor(recorder),
		Categories:    categories.NewHandler(categorySvc, log),
		Products:      products.NewHandler(productSvc, log),
		Admin:         admin.NewHandler(admin.NewService(pool, authSvc), log),
		Addresses:     addresses.NewHandler(addresses.NewService(pool), log),
		Orders:        orders.NewHandler(orderSvc, log),
		Notifications: notifications.NewHandler(pool, log),
		Store:         store.NewHandler(pool, log).WithLocation(storeLocation),
		CMS:           cms.NewHandler(cms.NewService(pool, storage, log), log),
		Payments:      paymentsHandler,
		Analytics:     analytics.NewHandler(analytics.NewService(pool, storeLocation), log),
		Zones:         delivery.NewZoneHandler(pool, log),
		Deliveries:    delivery.NewHandler(pool, log),
		LocalFiles:    storage.Handler(),
	}

	if cfg.EmailWorkerEnabled {
		var sender email.Sender = email.NewLogSender(log, !cfg.IsProduction())
		if cfg.ResendAPIKey != "" && cfg.EmailFrom != "" {
			sender = email.NewResendSender(cfg.ResendAPIKey, cfg.EmailFrom)
		}
		worker := notifications.NewWorker(pool, sender, log)
		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			worker.Run(ctx)
		}()
		defer func() {
			stop()
			<-workerDone
		}()
	}

	go orderSvc.RunExpiry(ctx, cfg.PaymentExpiry)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, log, handlers),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api listening", "port", cfg.Port, "env", cfg.AppEnv)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

// newLimiter shares rate-limit counters through Redis when it is configured and
// otherwise counts in memory. If Redis is down requests are allowed, not blocked.
func newLimiter(ctx context.Context, cfg config.Config, log *slog.Logger) (ratelimit.Limiter, *redis.Client) {
	if cfg.RedisURL == "" {
		log.Info("REDIS_URL is not set, rate limits are counted in memory")
		return ratelimit.NewMemory(), nil
	}
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Error("REDIS_URL is not valid, rate limits are counted in memory", "error", err)
		return ratelimit.NewMemory(), nil
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		log.Warn("Redis is not reachable, rate limiting will allow requests until it is", "error", err)
	}
	return ratelimit.NewRedis(client, "sati:"), client
}

// registerGauges publishes queue sizes, which are read from the database on every scrape.
func registerGauges(pool *pgxpool.Pool) {
	collect := func(query string) func(context.Context) ([]metrics.Sample, error) {
		return func(ctx context.Context) ([]metrics.Sample, error) {
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			rows, err := pool.Query(ctx, query)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var out []metrics.Sample
			for rows.Next() {
				var label string
				var n float64
				if err := rows.Scan(&label, &n); err != nil {
					return nil, err
				}
				out = append(out, metrics.Sample{Labels: []string{label}, Value: n})
			}
			return out, rows.Err()
		}
	}
	metrics.Default.GaugeFunc("sati_email_jobs", "Queued emails by status.", []string{"status"},
		collect(`SELECT status::text, count(*) FROM email_jobs GROUP BY status`))
	metrics.Default.GaugeFunc("sati_open_orders", "Orders still being worked, by status.", []string{"status"},
		collect(`SELECT status::text, count(*) FROM orders WHERE status IN ('PENDING','CONFIRMED','PREPARING','READY','OUT_FOR_DELIVERY') GROUP BY status`))
	metrics.Default.GaugeFunc("sati_payments_needing_refund", "Payments waiting for a refund.", []string{"provider"},
		collect(`SELECT provider, count(*) FROM payments WHERE needs_refund GROUP BY provider`))
}

type redisPinger struct{ client *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.client.Ping(ctx).Err() }

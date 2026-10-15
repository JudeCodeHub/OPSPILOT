// Command migrate applies the database migrations, inserts the seed data if
// the database is empty, and exits. It is safe to run repeatedly.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/git4jude/opspilot/services/internal/db"
	"github.com/git4jude/opspilot/services/internal/db/seed"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

func main() {
	base := config.LoadBase("migrate")
	log := logging.New(base.ServiceName, base.LogLevel)
	databaseURL := config.Required("DATABASE_URL")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	fail := func(msg string, err error) {
		log.Error(msg, "error", err.Error())
		os.Exit(1)
	}

	retry := deps.Retry{Attempts: base.StartupRetries, Delay: base.StartupRetryDelay}
	pool, err := deps.Postgres(ctx, databaseURL, 2, retry)
	if err != nil {
		fail("startup failed", err)
	}
	defer pool.Close()
	log.Info("running migrations", "db_host", pool.Config().ConnConfig.Host, "db_name", pool.Config().ConnConfig.Database)

	if err := db.Migrate(ctx, pool, log); err != nil {
		pool.Close()
		fail("migration failed", err)
	}
	if _, err := seed.Run(ctx, pool, log); err != nil {
		pool.Close()
		fail("seeding failed", err)
	}
	log.Info("database is up to date")
}

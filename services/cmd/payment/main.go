// Command payment is a fake card processor.
package main

import (
	"errors"

	"github.com/git4jude/opspilot/services/internal/payment"
	"github.com/git4jude/opspilot/services/internal/platform/app"
)

func main() {
	a := app.New("payment", errors.New("payment processor error: upstream connection reset"))

	processor := payment.NewProcessor()
	handler := payment.NewHandler(processor, a.Metrics)
	handler.Register(a.Router)
	a.Chaos.OnFailure = handler.RecordError

	go processor.RunSweeper(a.Ctx)

	a.Chaos.Start(a.Ctx)
	a.Run("idempotency_ttl", payment.IdempotencyTTL.String())
}

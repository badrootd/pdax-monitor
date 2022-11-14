package pg

import (
	"context"
	"encoding/json"
	"time"

	monitor "github.com/pudgydoge/pdax-monitor"
	"go.opencensus.io/trace"
)

// tradeRepository is a service for managing Trades.
type tradeRepository struct {
	client *Client
}

// Insert inserts trade's information in the repository.
func (r *tradeRepository) Insert(ctx context.Context, t *monitor.Trade) error {
	_, span := trace.StartSpan(ctx, "tradeRepository.Insert")
	defer span.End()

	_, err := r.client.db.ExecContext(
		ctx,
		r.client.tradeQ["insert"],
		t.CurrencyPair,
		t.Price,
		t.Quantity,
		t.Timestamp,
	)

	return err
}

// orderRepository is a service for managing orders.
type orderRepository struct {
	client *Client
}

// Insert inserts trade's information in the repository.
func (r *orderRepository) Insert(ctx context.Context, orders []monitor.Order) error {
	_, span := trace.StartSpan(ctx, "orderRepository.Insert")
	defer span.End()

	json, _ := json.Marshal(orders)

	_, err := r.client.db.ExecContext(
		ctx,
		r.client.orderQ["insert"],
		time.Now(),
		string(json),
	)

	return err
}

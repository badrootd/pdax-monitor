package monitor

import (
	"context"
	"time"

	"github.com/cockroachdb/apd"
)

// Trade is data from trade panel.
type Trade struct {
	CurrencyPair string
	Price        *apd.Decimal
	Quantity     *apd.Decimal
	Timestamp    time.Time
}

// OrderBook is a set of orders.
type OrderBook interface {
	Apply(update OrderBookUpdate)
	Orders() []Order
}

// OrderBookUpdate represents single order insert, update or removal.
type OrderBookUpdate struct {
	Insert   Order
	Update   OrderUpdate
	Remove   bool
	OldIndex int
	NewIndex int
}

// Order represents fetched order from PDAX order book panel.
type Order struct {
	Price       float64
	PriceDec    uint8
	Quantity    float64
	QuantityDec uint8
	Timestamp   float64
	Side        uint8
}

// OrderUpdate represents single order update.
type OrderUpdate struct {
	Quantity  float64
	Timestamp float64
}

// TradeRepository is a storage for fetched trades.
type TradeRepository interface {
	// Insert creates a trade's information new record in the repository.
	Insert(ctx context.Context, a *Trade) error
}

// OrderRepository is a storage for order book.
type OrderRepository interface {
	// Insert creates a order's information new record in the repository.
	Insert(ctx context.Context, o []Order) error
}

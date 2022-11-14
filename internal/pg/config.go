package pg

import (
	"github.com/go-kit/log"
)

// NewClient returns a new Client backed by Postgres.
func NewClient(options ...ConfigOption) *Client {
	c := Client{
		logger:         log.NewNopLogger(),
		maxConnections: defaultMaxConnections,
		trade:          &tradeRepository{},
		order:          &orderRepository{},
	}

	for _, opt := range options {
		opt(&c)
	}

	c.trade.client = &c
	c.order.client = &c

	return &c
}

// ConfigOption configures the client.
type ConfigOption func(*Client)

// WithLogger configures a logger to debug interactions with Postgres.
func WithLogger(l log.Logger) ConfigOption {
	return func(c *Client) {
		c.logger = l
	}
}

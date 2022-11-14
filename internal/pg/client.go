package pg

import (
	"database/sql"

	"github.com/go-kit/log"

	// pg driver registers itself as being available to the database/sql package.
	_ "github.com/lib/pq"
	monitor "github.com/pudgydoge/pdax-monitor"
)

const (
	// defaultMaxConnections is the default number of maximum opened and idle connections.
	// If n <= 0, then there is no limit on the number of open connections.
	defaultMaxConnections = 0
)

// Client represents a client to the underlying PostgreSQL data store.
type Client struct {
	db             *sql.DB
	logger         log.Logger
	maxConnections int

	tradeQ map[string]string
	orderQ map[string]string

	trade *tradeRepository
	order *orderRepository
}

// Open connection to PostgreSQL.
func (c *Client) Open(dataSourceName string) error {
	var err error

	c.logger.Log("level", "debug", "msg", "connecting to db")

	if c.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return err
	}

	if err = c.db.Ping(); err != nil {
		return err
	}

	c.db.SetMaxOpenConns(c.maxConnections)
	c.logger.Log("level", "debug", "msg", "connected to db")

	c.defineQueries()

	return nil
}

func (c *Client) defineQueries() {
	c.tradeQ = map[string]string{
		"insert": `
			INSERT INTO trade (currency_pair, price, quantity, created_at) VALUES ($1, $2, $3, $4)
		`,
	}
	c.orderQ = map[string]string{
		"insert": `
			INSERT INTO order_book (created_at, order_book) VALUES ($1, $2)
		`,
	}
}

// Close closes PostgreSQL connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// Schema sets up the initial schema.
func (c *Client) Schema() error {
	_, err := c.db.Exec(Schema)
	return err
}

// TradeRepository returns current instance of tradeRepository interface.
func (c *Client) TradeRepository() monitor.TradeRepository {
	return c.trade
}

// OrderRepository returns current instance of tradeRepository interface.
func (c *Client) OrderRepository() monitor.OrderRepository {
	return c.order
}

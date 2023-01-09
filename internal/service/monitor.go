package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	monitor "github.com/pudgydoge/pdax-monitor"
	"github.com/pudgydoge/pdax-monitor/internal/auth"
	"github.com/pudgydoge/pdax-monitor/internal/trade"
	"github.com/pudgydoge/pdax-monitor/internal/websocket"
	"github.com/pudgydoge/pdax-monitor/internal/websocket/binary"
)

const (
	clockLayout                = "15:04"
	wsPageUpdate               = 35
	wsPageReset                = 36
	wsTradeViewID              = 4.0
	pdaxMaintenanceDurationMin = 35

	// wsOrderBookViewID = 16.0.
)

// MonitorService is a service to monitor trades and orderbooks.
type MonitorService struct {
	AuthService     auth.PDAXAuthService
	PDAXTradeURL    string
	TradeRepository monitor.TradeRepository
	OrderRepository monitor.OrderRepository
	Logger          log.Logger
	tradeReader     trade.Reader
	// orderBook       monitor.OrderBook
}

// NewMonitorService instantiates MonitorService.
func NewMonitorService(options ...ConfigOption) MonitorService {
	monitorService := MonitorService{}

	for _, opt := range options {
		opt(&monitorService)
	}

	return monitorService
}

// ConfigOption configures the service.
type ConfigOption func(service *MonitorService)

// WithLogger configures a logger to debug the service.
func WithLogger(l log.Logger) ConfigOption {
	return func(m *MonitorService) {
		m.Logger = l
	}
}

// WithAuthService configures a authentication service for the monitor service.
func WithAuthService(a auth.PDAXAuthService) ConfigOption {
	return func(r *MonitorService) {
		r.AuthService = a
	}
}

// WithCurrencyCodes configures currency codes for traderReader.
func WithCurrencyCodes(cc map[int]string) ConfigOption {
	return func(r *MonitorService) {
		r.tradeReader = trade.Reader{
			CurrencyCodes: cc,
		}
	}
}

// WithTradeURL configures tradeURL for monitor service.
func WithTradeURL(url string) ConfigOption {
	return func(r *MonitorService) {
		r.PDAXTradeURL = url
	}
}

// WithTradeRepository configures trade repository for monitor service.
func WithTradeRepository(rep monitor.TradeRepository) ConfigOption {
	return func(r *MonitorService) {
		r.TradeRepository = rep
	}
}

// WithOrderRepository configures order repository for monitor service.
func WithOrderRepository(rep monitor.OrderRepository) ConfigOption {
	return func(r *MonitorService) {
		r.OrderRepository = rep
	}
}

// MonitorWithRecovery schedules monitor process with recovery scenarios.
func (m *MonitorService) MonitorWithRecovery(ctx context.Context, wsInitBook websocket.InitBook) error {
	for {
		err := m.MonitorTrades(ctx, wsInitBook)
		if err != nil {
			level.Warn(m.Logger).Log("msg", "pdax trade monitoring has been interrupted", "err", err)

			if isPDAXMaintenanceWindow() {
				// wait till maintenance window ends
				level.Info(m.Logger).Log("msg", "pdax is under maintenance, wait 35 minutes")
				time.Sleep(pdaxMaintenanceDurationMin * time.Minute)
				level.Info(m.Logger).Log("msg", "pdax maintenance should have ended, resume monitoring")
				continue
			}

			level.Info(m.Logger).Log("msg", "waiting 15 minutes and then restart monitoring")
			time.Sleep(15 * time.Minute)
			continue
		}

		return nil
	}
}

// MonitorTrades monitors websocket messages for trades and orders.
func (m *MonitorService) MonitorTrades(ctx context.Context, wsInitBook websocket.InitBook) error {
	authToken, err := m.AuthService.Login()
	if err != nil {
		level.Error(m.Logger).Log("msg", "failed to get auth token", "err", err)
		return err
	}
	level.Debug(m.Logger).Log("msg", "successfully authorized to PDAX")

	tradeConn := websocket.NewPDAXWebSocket(m.PDAXTradeURL)
	err = tradeConn.Connect()
	if err != nil {
		return err
	}
	defer tradeConn.Close()
	level.Info(m.Logger).Log("msg", "successfully connected to PDAX", "url", m.PDAXTradeURL)

	err = tradeConn.Bootstrap(authToken, wsInitBook)
	if err != nil {
		return fmt.Errorf("failed to bootstrap websocket: %v", err)
	}

	var closed bool
	var data []byte
	for !closed {
		select {
		case <-ctx.Done():
			return nil // graceful termination
		default:
		}

		closed, data, err = tradeConn.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to read message from trade websocket: %v", err)
		}

		m.handleBinMessage(ctx, data)
	}

	return nil
}

func (m *MonitorService) handleBinMessage(ctx context.Context, data []byte) {
	rc := binary.ReadCursor{
		CurPos: 0,
		Data:   data,
	}

	// most read values are going to be skipped
	mtype := rc.ReadUint8()
	if mtype == wsPageUpdate || mtype == wsPageReset {
		rc.ReadUint32() // message_id
		rc.ReadUint16() // seq_number

		viewID := rc.ReadFloat64()
		if viewID == wsTradeViewID { // trades have viewID == 4
			m.handleTrade(ctx, &rc)
		}
		// Disabled due to error with ReadOrderBookUpdate method "panic: runtime error: index out of range [7] with length 7"
		// Also we do not use ordebook at all.
		// else if viewID == wsOrderBookViewID { // orderbooks have viewID == 16 (BTC), 21 (ETH)
		// if mtype == wsPageReset { // initial PDAXOrderBook
		// 	m.orderBook = order.ReadOrderBook(&rc)
		// 	m.OrderRepository.Insert(ctx, m.orderBook.Orders())
		// } else if mtype == wsPageUpdate { // OrderBook_change is update of existing orderbook
		// 	orderBookUpdates := order.ReadOrderBookUpdate(&rc)
		// 	for _, update := range orderBookUpdates {
		// 		m.orderBook.Apply(update)
		// 	}
		// }
		// }
	}
}

func (m *MonitorService) handleTrade(ctx context.Context, rc *binary.ReadCursor) {
	rc.ReadFloat64()              // page_id
	rc.ReadUint8()                // nullable check
	tradeCount := rc.ReadUint16() // tradeCount (always even)

	for t := uint16(0); t < tradeCount; t += 2 {
		rc.ReadUint8()              // insert
		if rc.ReadUint16() == 128 { // number == 'TimeSales_change'
			rc.ReadUint16() // length
			// RC stands for ReadCursor, _after(N) suffix means cursor position at N byte after read
			readTrade := m.tradeReader.ReadNullableTrade(rc)
			if err := m.TradeRepository.Insert(ctx, &readTrade); err != nil {
				level.Error(m.Logger).Log("msg", "error saving to db", "err", err)
			}
		}
	}
}

func isPDAXMaintenanceWindow() bool {
	start, _ := time.Parse(clockLayout, "22:55")
	end, _ := time.Parse(clockLayout, "23:35")
	clockNow, _ := time.Parse(clockLayout, time.Now().UTC().Format(clockLayout))

	return clockNow.After(start) && clockNow.Before(end)
}

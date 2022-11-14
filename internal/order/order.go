package order

import (
	"sync"

	monitor "github.com/pudgydoge/pdax-monitor"
	"github.com/pudgydoge/pdax-monitor/internal/websocket/binary"
)

const orderBookCapacity = 800

// PDAXOrderBook is set of dynamically changing orders.
type PDAXOrderBook struct {
	orders []monitor.Order
	lock   *sync.RWMutex
}

// NewOrderBook instantiates PDAXOrderBook.
func NewOrderBook() PDAXOrderBook {
	return PDAXOrderBook{
		make([]monitor.Order, 0, orderBookCapacity), // by now PDAX has approx 800 orders at a time
		&sync.RWMutex{},
	}
}

// Apply is used to update PDAXOrderBook with OrderBookUpdate (insert, update, remove).
func (ob *PDAXOrderBook) Apply(update monitor.OrderBookUpdate) {
	ob.lock.Lock()
	defer ob.lock.Unlock()

	if update.Remove { // remove
		ob.remove(update.OldIndex)
		return
	}

	if !(update.Insert == monitor.Order{}) { // insert
		ob.insert(update.NewIndex, update.Insert)
		return
	}

	if !(update.Update == monitor.OrderUpdate{}) { // update
		ob.update(update.OldIndex, update.NewIndex, update.Update)
		return
	}
}

// Orders returns slice to orders.
func (ob *PDAXOrderBook) Orders() []monitor.Order {
	return ob.orders
}

func (ob *PDAXOrderBook) insert(index int, order monitor.Order) {
	if len(ob.orders) == index {
		ob.orders = append(ob.orders, order)
	} else {
		ob.orders = append(ob.orders[:index+1], ob.orders[index:]...)
		ob.orders[index] = order
	}
}

func (ob *PDAXOrderBook) update(oldIndex int, newIndex int, update monitor.OrderUpdate) {
	oldOrder := ob.orders[oldIndex]
	oldOrder.Quantity = update.Quantity
	oldOrder.Timestamp = update.Timestamp
	if oldIndex != newIndex {
		ob.remove(oldIndex)
		ob.insert(newIndex, oldOrder)
	} else {
		ob.orders[oldIndex] = oldOrder
	}
}

func (ob *PDAXOrderBook) remove(index int) {
	ob.orders = append(ob.orders[:index], ob.orders[index+1:]...)
}

// ReadOrderBookUpdate used to parse OrderBookUpdate object from byte stream.
func ReadOrderBookUpdate(rc *binary.ReadCursor) []monitor.OrderBookUpdate {
	var orderBookUpdates []monitor.OrderBookUpdate
	rc.ReadFloat64() // page_id
	rc.ReadUint8()   // first_index nullable check
	// rc.ReadFloat64() // first_index, mostly null

	count := rc.ReadUint16() // count
	for i := uint16(0); i < count; i++ {
		var orders []monitor.Order
		if rc.ReadUint8() != 0 { // read insert if not null
			orders = readOrderBookInsert(rc)
		}

		var update monitor.OrderUpdate
		if rc.ReadUint8() != 0 { // read update message if not null
			update = readOrderUpdate(rc)
		}

		var removeID float64     // ID to remove
		if rc.ReadUint8() != 0 { // read removal message if not null
			removeID = rc.ReadFloat64()
		}

		var oldIndex float64
		if rc.ReadUint8() != 0 { // read oldIndex if not null
			oldIndex = rc.ReadFloat64() // not 0 for remove, update
		}

		var newIndex float64
		if rc.ReadUint8() != 0 { // read newIndex if not null
			newIndex = rc.ReadFloat64() // not 0 for insert, update
		}
		rc.ReadUint8() // animate

		if len(orders) > 0 {
			for _, order := range orders {
				orderBookUpdates = append(orderBookUpdates, monitor.OrderBookUpdate{Insert: order, NewIndex: int(newIndex)})
			}
		}

		if removeID != 0 {
			orderBookUpdates = append(orderBookUpdates, monitor.OrderBookUpdate{Remove: true, OldIndex: int(oldIndex)})
		}

		if !(update == monitor.OrderUpdate{}) {
			orderBookUpdates = append(orderBookUpdates, monitor.OrderBookUpdate{Update: update, OldIndex: int(oldIndex), NewIndex: int(newIndex)})
		}
	}

	return orderBookUpdates
}

// ReadOrderBook used to parse PDAXOrderBook object from byte stream.
func ReadOrderBook(rc *binary.ReadCursor) monitor.OrderBook {
	orderBook := NewOrderBook()

	rc.ReadFloat64()           // page_id
	rc.ReadFloat64()           // first_index
	rc.ReadUint8()             // animate
	if rc.ReadUint16() == 27 { // number == 'OrderBook_change'
		length := rc.ReadUint16() // length
		for t := uint16(0); t < length; t++ {
			rc.ReadFloat64()              // index
			rc.ReadFloat64()              // ID
			ts1 := rc.ReadFloat64()       // timestamp 1 part
			rc.ReadUint32()               // timestamp 2 part
			rc.ReadUint8()                // InstrumentMarket  nullable check
			rc.ReadFloat64()              // InstrumentMarket (currency)
			side := rc.ReadUint8()        // Side (bid == 1 or ask == 0)
			price := rc.ReadFloat64()     // Price
			priceDec := rc.ReadUint8()    // PriceDecimals
			quantity := rc.ReadFloat64()  // VisibleQunatity
			quantityDec := rc.ReadUint8() // QuantityDecimals
			rc.ReadUint8()                // Flags
			rc.ReadFloat64()              // Orders
			rc.ReadFloat64()              // GeneralInterest

			rc.Advance(2) // Tag length, mostly 0
			rc.Advance(2) // OBInfo length, mostly 0
			// tagLength := math.Min(rc.ReadUint16Float(), 50) // Tag length, mostly 0
			// rc.Advance(uint(tagLength))
			//
			// OBInfoLength := math.Min(rc.ReadUint16Float(), 30) // OBInfo length, mostly 0
			// rc.Advance(uint(OBInfoLength))

			rc.ReadFloat64() // Currency
			rc.ReadFloat64() // TransactionCount
			rc.ReadUint32()  // permissions, RC_after(133)

			order := monitor.Order{
				Price:       price,
				PriceDec:    priceDec,
				Quantity:    quantity,
				QuantityDec: quantityDec,
				Timestamp:   ts1,
				Side:        side,
			}

			orderBookUpdate := monitor.OrderBookUpdate{Insert: order, NewIndex: int(t)}
			orderBook.Apply(orderBookUpdate)
		}
	}

	return &orderBook
}

func readOrderBookInsert(rc *binary.ReadCursor) []monitor.Order {
	var changes []monitor.Order
	if rc.ReadUint16() == 27 { // number == 'OrderBook_change'
		length := rc.ReadUint16() // length
		changes = make([]monitor.Order, length)

		for t := uint16(0); t < length; t++ {
			rc.ReadFloat64()              // index
			rc.ReadFloat64()              // ID
			ts1 := rc.ReadFloat64()       // timestamp 1 part
			rc.ReadUint32()               // timestamp 2 part
			rc.ReadUint8()                // InstrumentMarket  nullable check
			rc.ReadFloat64()              // InstrumentMarket (currency)
			side := rc.ReadUint8()        // Side (bid or ask)
			price := rc.ReadFloat64()     // Price
			priceDec := rc.ReadUint8()    // PriceDecimals
			quantity := rc.ReadFloat64()  // VisibleQunatity
			quantityDec := rc.ReadUint8() // QuantityDecimals
			rc.ReadUint8()                // Flags
			rc.ReadFloat64()              // Orders
			rc.ReadFloat64()              // GeneralInterest

			rc.Advance(2) // Tag length, mostly 0
			rc.Advance(2) // OBInfo length, mostly 0
			// tagLength := math.Min(rc.ReadUint16Float(), 50) // Tag length, mostly 0
			// rc.Advance(uint(tagLength))
			//
			// OBInfoLength := math.Min(rc.ReadUint16Float(), 30) // OBInfo length, mostly 0
			// rc.Advance(uint(OBInfoLength))

			rc.ReadFloat64() // Currency
			rc.ReadFloat64() // TransactionCount
			rc.ReadUint32()  // permissions, RC_after(133)

			changes[t] = monitor.Order{
				Price:       price,
				PriceDec:    priceDec,
				Quantity:    quantity,
				QuantityDec: quantityDec,
				Timestamp:   ts1,
				Side:        side,
			}
		}
	}

	return changes
}

func readOrderUpdate(rc *binary.ReadCursor) monitor.OrderUpdate {
	rc.ReadUint16() // table

	// foreign type ColumnKVPList
	rc.ReadUint16()          // table
	count := rc.ReadUint16() // count
	for i := uint16(0); i < count; i++ {
		rc.ReadUint8() // column_indices
	}
	rc.ReadFloat64()             // ID
	ts1 := rc.ReadFloat64()      // timestamp 1 part
	rc.ReadUint32()              // timestamp 2 part
	quantity := rc.ReadFloat64() // VisibleQuantity
	rc.ReadFloat64()             // Orders

	return monitor.OrderUpdate{
		Quantity:  quantity,
		Timestamp: ts1,
	}
}

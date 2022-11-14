package trade

import (
	"math"
	"time"

	"github.com/cockroachdb/apd"
	monitor "github.com/pudgydoge/pdax-monitor"
	"github.com/pudgydoge/pdax-monitor/internal/websocket/binary"
)

// Reader represents trade object reader.
type Reader struct {
	CurrencyCodes map[int]string
}

// ReadTrade used to parse Trade object from byte stream.
func (tr Reader) ReadTrade(rc *binary.ReadCursor) monitor.Trade {
	rc.ReadFloat64()        // index
	rc.ReadFloat64()        // ID
	ts1 := rc.ReadFloat64() // timestamp 1 part
	rc.ReadUint32()         // timestamp 2 part
	rc.ReadUint8()          // InstrumentMarket nullable check

	currency := rc.ReadFloat64() // InstrumentMarket (currency)
	price := rc.ReadFloat64()    // Price
	quantity := rc.ReadFloat64() // Quantity

	rc.ReadFloat64() // Value (for history fetching), Increment (for live fetching)
	rc.ReadFloat64() // Increment (for history fetching), Value (for live fetching)

	rc.ReadUint8()   // Aggressor
	rc.ReadFloat64() // Swing

	priceDec := rc.ReadUint8()    // PriceDecimals
	quantityDec := rc.ReadUint8() // QuantityDecimals, RC_after(111)
	rc.ReadUint8()                // ValueDecimals
	rc.ReadUint8()                // LeverageEvent
	rc.ReadUint32()               // permissions

	return monitor.Trade{
		CurrencyPair: tr.CurrencyCodes[int(currency)] + "-PHP",
		Price:        apd.New(int64(price), -int32(priceDec)),
		Quantity:     apd.New(int64(quantity), -int32(quantityDec)),
		Timestamp:    DecodeTime(ts1),
	}
}

// ReadNullableTrade used to parse nullable (for live monitoring) Trade object from byte stream.
func (tr Reader) ReadNullableTrade(rc *binary.ReadCursor) monitor.Trade {
	// Trade main part, order should be preserved
	trade := tr.ReadTrade(rc)

	// Trade second null part
	rc.ReadUint8()   // update, nullable check
	rc.ReadUint8()   // remove, nullable check, RC_after(119)
	rc.ReadUint8()   // old_index, nullable check
	rc.ReadUint8()   // new_index, nullable check
	rc.ReadFloat64() // new_index
	rc.ReadUint8()   // animate, , RC_after(130)

	rc.ReadUint8()   // insert, nullable check
	rc.ReadUint8()   // update, nullable check
	rc.ReadUint8()   // remove, nullable check
	rc.ReadFloat64() // remove
	rc.ReadUint8()   // old_index, nullable check
	rc.ReadFloat64() // old_index, RC_after(150)
	rc.ReadUint8()   // new_index, nullable check
	rc.ReadUint8()   // animate

	return trade
}

// DecodeTime is to parse binary encoded (not sure its standard, might be proprietary) timestamp.
func DecodeTime(f float64) time.Time {
	t := math.Floor(f/86400) + 730425

	n := signedFloor(withT(t) / 146097)
	r := t - 146097*n
	i := signedFloor((r - signedFloor(r/1460) + signedFloor(r/36524) - signedFloor(r/146096)) / 365)
	a := r - (365*i + signedFloor(i/4) - signedFloor(i/100))
	s := signedFloor((5*a + 2) / 153)
	o := s + withS(s)

	year := int(i + 400*n + withO(o))
	month := int(o)
	day := int(a - signedFloor((153*s+2)/5) + 1)
	hour := int(math.Mod(signedFloor(f/3600), 24))
	minute := int(math.Mod(signedFloor(f/60), 60))
	second := int(signedFloor(math.Mod(f, 60)))

	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
}

func withO(o float64) float64 {
	if o <= 2 {
		return 1
	}

	return 0
}

func signedFloor(s float64) float64 {
	if s < 0 {
		return math.Ceil(s)
	}

	return math.Floor(s)
}

func withS(s float64) float64 {
	if s < 10 {
		return 3
	}

	return -9
}

func withT(t float64) float64 {
	if t >= 0 {
		return t
	}

	return t - 146096
}

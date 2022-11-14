package main

import (
	"encoding/base64"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-kit/log"
	"github.com/spf13/viper"
	monitor "github.com/pudgydoge/pdax-monitor"
	"github.com/pudgydoge/pdax-monitor/internal/auth"
	"github.com/pudgydoge/pdax-monitor/internal/trade"
	"github.com/pudgydoge/pdax-monitor/internal/websocket"
	"github.com/pudgydoge/pdax-monitor/internal/websocket/binary"
)

const (
	pdaxTradeURL              = "wss://trade.pdax.ph/tradeui/ws/master"
	defaultPDAXAuthRefreshURL = "https://trade.pdax.ph/moon/v1/refreshToken"
	pdaxTradeHistoryFile      = "volume-pdax-history.csv"
	pdaxAuthURL               = "https://trade.pdax.ph/moon/v1/login"
	pdaxSignInURL             = "https://trade.pdax.ph/signin"
	pdaxCaptchaGoogleKey      = "6Lcj_WQUAAAAAH7U8sEordiEHPEJDdVzoKQiH7Oa"
)

func main() {
	var err error
	var bookPath string
	var currencyCodesPath string
	var pdaxUsername string
	var pdaxPassword string
	var solverServiceKey string
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&bookPath, "wsInitBook", "./auxiliary/wsbook.json", "Path to the PDAX websocket connection bootstrap book")
	fs.StringVar(&currencyCodesPath, "currencyCodes", "./auxiliary/currencyCodes.json", "Path to the PDAX currency codes file")
	fs.StringVar(&pdaxUsername, "pdax.username", "pdax-username", "PDAX username")
	fs.StringVar(&pdaxPassword, "pdax.password", "pdax-password", "PDAX password")
	fs.StringVar(&solverServiceKey, "captcha.solver-key", "captcha-solver-key", "Recaptcha solver key")

	wsInitBook := unmarshalWsInitBook(bookPath)
	currencyCodes := unmarshalCurrencyCodes(currencyCodesPath)
	durationHours := fs.Int("duration", 24, "Fetch trades for the last duration hours before now (defaults 24 hours)")

	captchaSolver := auth.CaptchaSolver{
		SolverKey:   solverServiceKey,
		TaskKey:     pdaxCaptchaGoogleKey,
		TaskPageURL: pdaxSignInURL,
		Logger:      log.NewNopLogger(),
	}
	authService := auth.NewAuthService(
		pdaxAuthURL,
		defaultPDAXAuthRefreshURL,
		auth.WithCaptchaSolver(captchaSolver),
		auth.WithLogger(log.NewNopLogger()),
		auth.WithCredentials(pdaxUsername, pdaxPassword),
	)

	since := time.Now().Add(time.Duration(-*durationHours) * time.Hour)

	tradeReader := trade.Reader{
		CurrencyCodes: currencyCodes,
	}
	err = fetchTrades(authService, since, wsInitBook, tradeReader)
	if err != nil {
		fmt.Printf("Failed to get trades %v", err)
		return
	}
}

func unmarshalCurrencyCodes(currencyCodesPath string) map[int]string {
	currencyCodes := make(map[int]string)
	if _, err := os.Stat(currencyCodesPath); !os.IsNotExist(err) {
		viper.SetConfigFile(currencyCodesPath)
		err = viper.ReadInConfig()
		if err != nil {
			fmt.Printf("failed to read currency codes file: %v", err)
			return map[int]string{}
		}

		for key, value := range viper.GetStringMapString("currencyCodes") {
			keyInt, _ := strconv.Atoi(key)
			currencyCodes[keyInt] = value
		}
	}

	return currencyCodes
}

func unmarshalWsInitBook(bookPath string) websocket.InitBook {
	var wsInitBook websocket.InitBook
	if _, err := os.Stat(bookPath); !os.IsNotExist(err) {
		viper.SetConfigFile(bookPath)
		err = viper.ReadInConfig()
		if err != nil {
			fmt.Printf("failed to read binary book: %v", err)
			return websocket.InitBook{}
		}

		err = viper.Unmarshal(&wsInitBook)
		if err != nil {
			fmt.Printf("unmarshaling binary book failed: %v", err)
			return websocket.InitBook{}
		}
	}

	return wsInitBook
}

func fetchTrades(authService auth.PDAXAuthService, since time.Time, wsInitBook websocket.InitBook, tradeReader trade.Reader) error {
	authToken, err := authService.Login()
	if err != nil {
		fmt.Printf("Failed to get auth token %v", err)
		return err
	}
	fmt.Printf("Successfully signed in to %s\nStart recording trades...\n", pdaxAuthURL)

	tradeHistoryFile, err := os.OpenFile(pdaxTradeHistoryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)

	defer func() {
		err := tradeHistoryFile.Close()
		if err != nil {
			fmt.Printf("failed to close trade history output file: %v\n", err)
		}
	}()

	tradeOutput := csv.NewWriter(tradeHistoryFile)
	if err != nil {
		fmt.Printf("Failed to open trade csv output file: %v\n", err)
		return err
	}
	stat, err := tradeHistoryFile.Stat()
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		tradeOutput.Write([]string{"CurrencyPair", "Price", "Quantity", "TimeGMT+3", "Timestamp"})
		tradeOutput.Flush()
	}

	tradeConn := websocket.NewPDAXWebSocket(pdaxTradeURL)
	err = tradeConn.Connect()
	if err != nil {
		return err
	}
	defer tradeConn.Close()

	err = tradeConn.Bootstrap(authToken, wsInitBook)
	if err != nil {
		return err
	}

	// 10000 is enough to fetch all PDAX provided trades, by now its only about 6600 records
	var pageSize uint16 = 10000 // pageSize is last two bytes in second 40 byte message
	requestPage(&tradeConn, pageSize, -1)

	// We requested last pageSize count trades, now lets search for response message from websocket
	var closed bool
	var data []byte
	for !closed {
		closed, data, err = tradeConn.ReadMessage()
		if err != nil {
			return err
		}

		// pageSize count historical trades are always greater than 50000 bytes, skip other messages
		// we do such filtering to avoid unexpected message format changes
		if len(data) > 50000 {
			// Messages are deserialized in a way PDAX does it. Yes, they do it themselves
			// I dont know why PDAX do not use ready libs or framework for it
			rc := binary.ReadCursor{
				CurPos: 0,
				Data:   data,
			}

			// most read values are going to be skipped
			if rc.ReadUint8() == 36 { // historical trades have type == 36 (PageResetMessage)
				rc.ReadUint32() // message_id
				rc.ReadUint16() // seq_number

				if rc.ReadFloat64() == 4.0 { // trades have view_id == 4
					rc.ReadFloat64() // page_id
					rc.ReadFloat64() // first_index
					rc.ReadUint8()   // animate

					if rc.ReadUint16() == 128 { // number == 'TimeSales_change'
						fetchedLength := rc.ReadUint16()
						for i := uint16(0); i < fetchedLength; i++ {
							fetchedTrade := tradeReader.ReadTrade(&rc)

							if fetchedTrade.Timestamp.Before(since) { // we hit limit, exit
								fmt.Println("Finished fetching trade history")
								return nil
							}

							WriteTradeToFile(fetchedTrade, tradeOutput)
						}

						return nil
					}
				}
			}
		}
	}

	return nil
}

func requestPage(conn *websocket.PDAXWebsocket, pageSize uint16, sinceID float64) {
	parameters := make([]byte, 11)

	wc := binary.WriteCursor{
		CurPos: 0,
		Data:   parameters,
	}
	wc.WriteFloat64(sinceID)
	wc.WriteUint8(0x01)
	wc.WriteUint16(pageSize)
	fetchMessage, _ := base64.StdEncoding.DecodeString("HAAAAAYAAACAAAAAAAAAAAEACVRpbWVzdGFtcP8=")

	err := conn.WriteMessage(append(fetchMessage, parameters...))
	if err != nil {
		fmt.Printf("PDAX websocket write error: %v\n", err)
	}
}

// WriteTradeToFile function to save trade to csv file.
func WriteTradeToFile(trade monitor.Trade, tradeOutput *csv.Writer) {
	price, _ := trade.Price.Float64()
	quantity, _ := trade.Quantity.Float64()
	err := tradeOutput.Write([]string{
		trade.CurrencyPair,
		fmt.Sprintf("%f", price),
		fmt.Sprintf("%f", quantity),
		trade.Timestamp.Add(3 * time.Hour).Format("2006-01-02 15:04:05"), // Moscow time
		fmt.Sprintf("%d", trade.Timestamp.Unix()),
	})
	if err != nil {
		fmt.Println("Failed to write trade to file")
	}
	tradeOutput.Flush()
}

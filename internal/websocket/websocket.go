package websocket

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// PDAXWebsocket is PDAX adapted wrapper upon gorilla websocket.
type PDAXWebsocket struct {
	PDAXTradeURL string
	conn         *websocket.Conn
}

// NewPDAXWebSocket instantiates PDAXWebsocket.
func NewPDAXWebSocket(pdaxTradeURL string) PDAXWebsocket {
	return PDAXWebsocket{
		PDAXTradeURL: pdaxTradeURL,
	}
}

// Connect is lib connect call wrapper.
func (ws *PDAXWebsocket) Connect() error {
	var err error

	var wsDialer = &websocket.Dialer{
		HandshakeTimeout: 120 * time.Second,
	}

	ws.conn, _, err = wsDialer.Dial(ws.PDAXTradeURL, nil)

	if err != nil {
		return fmt.Errorf("pdax websocket connect error: %v", err)
	}

	return nil
}

// Bootstrap websocket connection with prerecorded messages to make PDAX backend start sending us trades and order books.
func (ws *PDAXWebsocket) Bootstrap(authToken string, wsInitBook InitBook) error {
	var err error
	// read and ignore PDAX.RouteInfoMessage
	_, _, err = ws.conn.ReadMessage()
	if err != nil {
		return err
	}

	err = ws.sendAuthMessage(authToken)
	if err != nil {
		return err
	}

	// mandatory response to PDAX.LoginReplyMessage: PDAX.MessageCallBackInfo
	messageCallbackInfo, _ := base64.StdEncoding.DecodeString(wsInitBook.MessageCallBackInfo)
	err = ws.conn.WriteMessage(websocket.BinaryMessage, messageCallbackInfo)
	if err != nil {
		return err
	}

	m0, _ := base64.StdEncoding.DecodeString(wsInitBook.M0)
	err = ws.conn.WriteMessage(websocket.BinaryMessage, m0)
	if err != nil {
		return err
	}

	for n := 0; n < 2; n++ {
		_, _, err = ws.conn.ReadMessage()
		if err != nil {
			return err
		}
	}

	m1, _ := base64.StdEncoding.DecodeString(wsInitBook.M1) // 28 bytes
	err = ws.conn.WriteMessage(websocket.BinaryMessage, m1)
	if err != nil {
		return fmt.Errorf("pdax websocket m1 write error: %v", err)
	}

	// Another reads-write
	for n := 0; n < 2; n++ {
		_, _, err = ws.conn.ReadMessage()
		if err != nil {
			return err
		}
	}

	i := 0
	for _, m := range wsInitBook.Messages {
		i++
		mc, _ := base64.StdEncoding.DecodeString(m)
		err = ws.conn.WriteMessage(websocket.BinaryMessage, mc)
		if err != nil {
			return fmt.Errorf("pdax replay wsInitBook websocket %d write error: %v", i, err)
		}
	}

	go ws.scheduleHeartbeat()

	return nil
}

func (ws *PDAXWebsocket) sendAuthMessage(authToken string) error {
	// send JWT refreshed AuthToken (got from gcaptcha)
	prefix, _ := base64.StdEncoding.DecodeString("BgAAAAEAAAG7")
	payload := []byte(authToken)
	err := ws.conn.WriteMessage(websocket.BinaryMessage, append(prefix, payload...))
	if err != nil {
		return err
	}

	_, _, err = ws.conn.ReadMessage()
	if err != nil {
		return err
	}

	return nil
}

// schedule pdax application heartbeats to avoid connection close from backend app.
func (ws *PDAXWebsocket) scheduleHeartbeat() error {
	var err error
	time.Sleep(5 * time.Second) // do not send it too early
	i := 0
	for {
		err = ws.conn.WriteMessage(websocket.BinaryMessage, []byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		if err != nil {
			err = fmt.Errorf("failed to send heartbeat: %v", err)
			break
		}
		time.Sleep(15 * time.Second)
		i++
	}

	return err
}

// ReadMessage is lib message read call wrapper.
func (ws *PDAXWebsocket) ReadMessage() (bool, []byte, error) {
	mtype, data, err := ws.conn.ReadMessage()

	return mtype == websocket.CloseMessage, data, err
}

// WriteMessage is lib write call wrapper.
func (ws *PDAXWebsocket) WriteMessage(message []byte) error {
	return ws.conn.WriteMessage(websocket.BinaryMessage, message)
}

// Close is lib close call wrapper.
func (ws *PDAXWebsocket) Close() error {
	return ws.conn.Close()
}

// InitBook represents prerecorded stream of messages to init websocket connection.
type InitBook struct {
	MessageCallBackInfo string   `json:"messageCallbackInfo"`
	M0                  string   `json:"m0"`
	M1                  string   `json:"m1"`
	Messages            []string `json:"messages"`
}

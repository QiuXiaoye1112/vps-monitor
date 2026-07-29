package connection

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const writeTimeout = 30 * time.Second

type SafeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
	ID   int64
}

func NewSafeConn(conn *websocket.Conn) *SafeConn {
	return &SafeConn{
		conn: conn,
		mu:   sync.Mutex{},
		ID:   time.Now().UnixNano(),
	}
}

func (sc *SafeConn) WriteMessage(messageType int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if err := sc.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	err := sc.conn.WriteMessage(messageType, data)
	_ = sc.conn.SetWriteDeadline(time.Time{})
	return err
}

func (sc *SafeConn) WriteJSON(v interface{}) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if err := sc.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	err := sc.conn.WriteJSON(v)
	_ = sc.conn.SetWriteDeadline(time.Time{})
	return err
}

func (sc *SafeConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	// Gorilla permits WriteControl concurrently with all other methods. Keeping
	// it outside the data-writer mutex lets heartbeat Pongs bypass a stalled
	// report write.
	return sc.conn.WriteControl(messageType, data, deadline)
}

func (sc *SafeConn) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.Close()
}
func (sc *SafeConn) ReadMessage() (int, []byte, error) {
	return sc.conn.ReadMessage()
}
func (sc *SafeConn) ReadJSON(v interface{}) error {
	return sc.conn.ReadJSON(v)
}
func (sc *SafeConn) SetReadDeadline(t time.Time) error {
	return sc.conn.SetReadDeadline(t)
}
func (sc *SafeConn) SetPingHandler(h func(appData string) error) {
	sc.conn.SetPingHandler(h)
}
func (sc *SafeConn) GetConn() *websocket.Conn {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn
}

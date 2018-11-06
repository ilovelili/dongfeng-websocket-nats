package websocketnats

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionID connection id
type ConnectionID int64

// UserID user id
type UserID string

// DeviceID device id.
// Regarding to device, although the best approach is to get the device info by parsing the JWT provided by Auth0, I am not very sure if Auth0 provides device info scope.
// So, we fallback to IP if deviceID not saved in JWT
type DeviceID string

// Connection wraps websocket connection.
type Connection struct {
	ws            *websocket.Conn
	id            ConnectionID
	userID        UserID
	deviceID      DeviceID
	startTime     time.Time
	lastMessageAt time.Time
	dataMutex     sync.RWMutex
	writeMutex    sync.Mutex
}

// NewConnection init the connection
func NewConnection(id ConnectionID, ws *websocket.Conn) *Connection {
	c := &Connection{
		ws:         ws,
		id:         id,
		userID:     "",
		deviceID:   "",
		startTime:  time.Now(),
		dataMutex:  sync.RWMutex{},
		writeMutex: sync.Mutex{},
	}
	return c
}

// ReadMessage read
func (c *Connection) ReadMessage() (messageType int, p []byte, err error) {
	return c.ws.ReadMessage()
}

// SendText write text
func (c *Connection) SendText(message []byte) {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.ws.WriteMessage(websocket.TextMessage, message)
}

// SendBinary write binary
func (c *Connection) SendBinary(message []byte) {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.ws.WriteMessage(websocket.BinaryMessage, message)
}

// Close close the connection and set connection id to -1
func (c *Connection) Close(code int, reason string) {
	c.dataMutex.Lock()
	c.dataMutex.Unlock()
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason))
	c.ws.Close()

	c.id = -1
	c.userID = ""
	c.deviceID = ""
}

// IsLoggedIn check if logged in or not by userID in the connection
func (c *Connection) IsLoggedIn() bool {
	c.dataMutex.RLock()
	defer c.dataMutex.RUnlock()

	return c.userID != ""
}

// IsClosed check connection closed or not
func (c *Connection) IsClosed() bool {
	c.dataMutex.RLock()
	defer c.dataMutex.RUnlock()

	return c.IsClosed()
}

// GetInfo get connection id, user id, device id from connection
func (c *Connection) GetInfo() (ConnectionID, UserID, DeviceID) {
	c.dataMutex.RLock()
	defer c.dataMutex.RUnlock()

	return c.id, c.userID, c.deviceID
}

// GetStartTime get connection start time
func (c *Connection) GetStartTime() time.Time {
	c.dataMutex.RLock()
	defer c.dataMutex.RUnlock()

	return c.startTime
}

// Login login using user id and device id
func (c *Connection) Login(userID UserID, deviceID DeviceID) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	c.userID = userID
	c.deviceID = deviceID
	c.ws.SetReadLimit(0)
}

// UpdateLastPingTime update last message ping time
func (c *Connection) UpdateLastPingTime() {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	c.lastMessageAt = time.Now()
}

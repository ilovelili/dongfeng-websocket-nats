// Package websocketnats One-way websocket gateway for nats.
// limitations:
// . Does not support sending data to websocket server except login request
// . Does not support protobuf
// . Does not support websocket binary reading / sending
// The unsupported features can be easily added into the lib if we need rich websocket functionalities
package websocketnats

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	nats "github.com/nats-io/go-nats"
)

// Config configurations of nats websocket gateway
type Config struct {
	ListenInterface string   `json:"listenInterface"`
	URLPattern      string   `json:"urlPattern"`
	JWKS            string   `json:"jwks"`
	NatsAddress     string   `json:"natsAddress"`
	NatsPoolSize    int      `json:"natsPoolSize"`
	NatsTopics      []string `json:"natsTopics"`
	RemoteAddr      string   `json:"remoteAddr"`
}

// MessageType Text or Binary
type MessageType int32

const (
	// Text Text
	Text MessageType = 0
	// Binary Binary
	Binary MessageType = 1
)

const (
	// LoginPrefix login prefix
	LoginPrefix = "login>:"

	// TopicPrefix message bus topic prefix
	TopicPrefix = "topic>:"
)

const (
	// MaxUnLoggedConnectionCount allow in the pool. If conection exceeds the threshold, the connections exceeds the UnLoggedConnectionTimeout will be closed
	MaxUnLoggedConnectionCount = 200
	// UnLoggedConnectionTimeout timeout in seconds for the un-logged in connections
	UnLoggedConnectionTimeout = 60
)

// NatsWebSocket Nats websocket entity. Including config, pool, server info and so on
type NatsWebSocket struct {
	config               *Config
	natsPool             *Pool
	httpServer           *http.Server
	upgrader             websocket.Upgrader
	connections          *ConnectionsStorage
	lastConnectionNumber int64
}

// New constructor
func New(config *Config) *NatsWebSocket {
	return &NatsWebSocket{
		config:      config,
		upgrader:    websocket.Upgrader{},
		connections: NewConnectionsStorage(),
	}
}

// Start init a nats connection pool and then start http server
func (w *NatsWebSocket) Start() error {
	stopSignal := getOsSignalWatcher()
	natsPool, err := NewPool(w.config.NatsAddress, w.config.NatsPoolSize)
	if err != nil {
		log.Panicf("can't connect to nats: %v", err)
	}

	w.natsPool = natsPool
	defer func() { natsPool.Empty() }()

	go func() {
		<-stopSignal
		w.Stop()
	}()

	return w.startHTTPServer()
}

// Stop shutdown http server and finalize nats connection pool
func (w *NatsWebSocket) Stop() {
	if w.httpServer != nil {
		w.httpServer.Shutdown(nil)
		log.Println("http: shutdown")
	}

	w.natsPool.Empty()
	log.Println("nats-pool: empty")
}

func (w *NatsWebSocket) getNewConnectionID() ConnectionID {
	return ConnectionID(atomic.AddInt64(&w.lastConnectionNumber, 1))
}

func (w *NatsWebSocket) registerConnection(connection *websocket.Conn) *Connection {
	wsConnection := NewConnection(w.getNewConnectionID(), connection)
	w.connections.AddNewConnection(wsConnection)

	connection.SetCloseHandler(func(code int, Text string) error {
		w.onClose(wsConnection)
		return nil
	})

	return wsConnection
}

func (w *NatsWebSocket) unregisterConnection(connection *Connection) {
	w.connections.RemoveConnection(connection)
}

func (w *NatsWebSocket) onConnection(writer http.ResponseWriter, request *http.Request) {
	connection, err := w.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}

	// sets the maximum size for a message read from the peer
	connection.SetReadLimit(1024) // Glory for hard coding!
	con := w.registerConnection(connection)

	// handle input
	go w.handleInputMessages(con)

	w.cleanConnectionsIfNeed(con)
}

func (w *NatsWebSocket) cleanConnectionsIfNeed(connection *Connection) {
	now := time.Now().Unix()
	stats := w.connections.GetStats()

	if stats.NumberOfNotLoggedConnections > MaxUnLoggedConnectionCount {
		w.connections.RemoveIf(func(con *Connection) bool {
			return now-con.GetStartTime().Unix() > UnLoggedConnectionTimeout
		}, func(con *Connection) {
			con.Close(websocket.ClosePolicyViolation, "Auth")
		})
	}
}

func (w *NatsWebSocket) handleInputMessages(connection *Connection) {
	for {
		messageType, message, err := connection.ReadMessage()
		if err != nil {
			connection.Close(websocket.CloseInternalServerErr, "ServerError")
			w.onClose(connection)
			return
		}

		connection.UpdateLastPingTime()

		switch messageType {
		case websocket.TextMessage:
			w.onTextMessage(connection, message)
		case websocket.BinaryMessage:
			w.onBinaryMessage(connection, message)
		case websocket.CloseMessage:
			w.onClose(connection)
			return
		}
	}
}

func (w *NatsWebSocket) onTextMessage(connection *Connection, message []byte) {
	// respond ping
	if bytes.Compare(message, []byte("ping")) == 0 {
		connection.SendText([]byte("pong"))
		return
	}

	isLoginMessage := bytes.HasPrefix(message, []byte(LoginPrefix))
	if isLoginMessage {
		w.login(connection, message[len(LoginPrefix):])
		return
	}

	isTopicMessage := bytes.HasPrefix(message, []byte(TopicPrefix))
	if isTopicMessage {
		if !connection.IsLoggedIn() {
			connection.SendText([]byte("go away"))
			return
		}

		// since logged in, we allow the connection subscribe to message bus
		w.setupSubsrciber(connection, message[len(TopicPrefix):])
		return
	}
}

// we don't support binary msg yet. But I leave the interface here. The implementation should be very easy
func (w *NatsWebSocket) onBinaryMessage(connection *Connection, message []byte) {
	connection.SendText([]byte("binary message is not supported yet"))
	return
}

func (w *NatsWebSocket) onClose(connection *Connection) {
	connectionID, _, _ := connection.GetInfo()
	if connectionID == -1 {
		return
	}

	w.unregisterConnection(connection)
}

func (w *NatsWebSocket) setupSubsrciber(connection *Connection, topic []byte) {
	// the topic is invalid
	if !contains(w.config.NatsTopics, string(topic)) {
		connection.SendText([]byte("invalid topic"))
		return
	}

	busClient, err := w.natsPool.Get()
	if err != nil {
		log.Fatalf("Can't connect to nats: %v", err)
		return
	}

	_, err = busClient.Subscribe(string(topic), func(msg *nats.Msg) {
		connection.SendText([]byte(msg.Data))
	})

	if err != nil {
		log.Fatalf("Can't connect to nats: %v", err)
		return
	}
}

// https://stackoverflow.com/questions/4361173/http-headers-in-websockets-client-api
// Can't assign JWT in request header. So send the explicit login request like login>:Bearer <id token>
func (w *NatsWebSocket) login(connection *Connection, tokenBinary []byte) {
	idtoken, valid := ResolveIDToken(string(tokenBinary))
	if !valid {
		connection.SendText([]byte(LoginPrefix + "Not Authorized"))
		return
	}

	claims, token, err := ParseJWT(idtoken, w.config.JWKS)
	if err != nil || !token.Valid {
		connection.SendText([]byte(LoginPrefix + "Not Authorized"))
		return
	}

	var userID UserID
	var deviceID DeviceID

	// fallback to user name if no user id found in claims
	if uid, ok := claims["userId"]; ok {
		userID = UserID(uid.(string))
	} else {
		userID = UserID(claims["name"].(string))
	}

	// fallback to remote ip if no device id found in claims
	// if did, ok := claims["deviceId"]; ok {
	// 	deviceID = DeviceID(did.(string))
	// } else {
	//	deviceID = DeviceID(w.config.RemoteAddr)
	// }
	deviceID = DeviceID(w.config.RemoteAddr)

	_, conUserID, _ := connection.GetInfo()

	if conUserID != "" {
		// user mismatch, which is not good
		if conUserID != userID {
			connection.SendText([]byte("go away"))
			return
		}

		connection.SendText([]byte("ok"))
		return
	}

	connection.Login(userID, deviceID)

	deviceConnectionBefore := w.connections.OnLogin(connection)
	if deviceConnectionBefore != nil {
		// purge the previous connection
		deviceConnectionBefore.Close(websocket.CloseGoingAway, "OneConnectionPerDevice")
		w.unregisterConnection(deviceConnectionBefore)
	}

	connection.SendText([]byte("ok"))
}

func (w *NatsWebSocket) startHTTPServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc(w.config.URLPattern, w.onConnection)
	srv := http.Server{
		Addr:    w.config.ListenInterface,
		Handler: mux,
	}

	w.httpServer = &srv

	log.Println("Start nats-http on: " + w.config.ListenInterface)
	return srv.ListenAndServe()
}

func getOsSignalWatcher() chan os.Signal {
	stopChannel := make(chan os.Signal)
	signal.Notify(stopChannel, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	return stopChannel
}

// contains check if string slice contains the element
func contains(source []string, target string) bool {
	for _, element := range source {
		if element == target {
			return true
		}
	}
	return false
}

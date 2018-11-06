package websocketnats

import (
	"fmt"
	"log"
	"sync"
	. "testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const (
	listeninterface = "localhost:8080"
	jwks            = "https://min.auth0.com/.well-known/jwks.json"
	natsaddress     = "nats://localhost:4222"
	natspoolsize    = 2
	natstopic       = "test.a"
)

// Please issue a new JWT to test. Since JWT expires
const (
	MockUser = "min"
	MockJWT  = "Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsImtpZCI6Ik1EWTNRVFV3TlRJek56VXhSVGcyUWpFM1JqWTFNRFF4UkRnMFJFWXhSRVJETmpNMlFqazVRdyJ9.eyJuaWNrbmFtZSI6Imlsb3ZlbGlsaSIsIm5hbWUiOiJtaW4ganUiLCJwaWN0dXJlIjoiaHR0cHM6Ly9hdmF0YXJzMi5naXRodWJ1c2VyY29udGVudC5jb20vdS8xOTU2NjEyP3Y9NCIsInVwZGF0ZWRfYXQiOiIyMDE4LTA0LTA2VDA3OjI3OjI1LjAzNVoiLCJlbWFpbCI6InJvdXRlNjY2QGxpdmUuY24iLCJpc3MiOiJodHRwczovL21pbi5hdXRoMC5jb20vIiwic3ViIjoiZ2l0aHVifDE5NTY2MTIiLCJhdWQiOiIxalZmOXdDcXV5dlRkM1JzYk1nRnZPYUFBZ0Z0dXB6UCIsImlhdCI6MTUyMjk5OTY2NSwiZXhwIjoxNTIzMDM1NjY1LCJhY3IiOiJodHRwOi8vc2NoZW1hcy5vcGVuaWQubmV0L3BhcGUvcG9saWNpZXMvMjAwNy8wNi9tdWx0aS1mYWN0b3IiLCJhbXIiOlsibWZhIl19.BRSOFCH6glLRif1b2ZbOp8l9rybnvEpVF06a9n6_S_yI4545B8KngBPBE4_KQ4SP7BfVhh2aIyGRuBDpp-pQzqV-QQsxfuk1qPNhsq3i2F1yMBbxpKHTFp4s8H907iHDmiVv1Ik5Qbk3SaMLMM-1hSyep2OgD5xzmgpZLrD0e80WJ-ejuGK8iY4JpZEVRa7wS3g2EUvHnNOjPQXFZyQSrm0zTuI6yA3VqZCwlkyj0dzweKtk5MlOu-Tb30AjqxEItHPLi-S8jZHuguRVcfGyikf82CnH3BHlqaE6q-NjTC6NHn5GdsqHVFlWT8heFMzK6UcWdWJIM-I8jVwy0ZYcWg"
)

var dialer = websocket.Dialer{}

func TestConnect(t *T) {
	server := startWsServer()
	wg := sync.WaitGroup{}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go startTestConnectConnection(t, i%2 == 0, &wg)
	}

	wg.Wait()
	server.Stop()
}

func startWsServer() *NatsWebSocket {
	natsWebsocket := New(&Config{
		ListenInterface: listeninterface,
		JWKS:            jwks,
		URLPattern:      "/",
		NatsAddress:     natsaddress,
		NatsPoolSize:    natspoolsize,
		NatsTopics:      []string{"test.a", "test.b"},
	})

	go func() {
		natsWebsocket.Start()
	}()

	time.Sleep(5 * time.Second)
	return natsWebsocket
}

func startTestConnectConnection(t *T, sendWrongToken bool, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, _, err := dialer.Dial("ws://"+listeninterface+"/", nil)
	assert.Nil(t, err)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	var token []byte

	if sendWrongToken {
		token = []byte("Bearer abcde")
	} else {
		token = []byte(MockJWT)
	}

	//test login
	err = conn.WriteMessage(websocket.TextMessage, []byte("login>:"+string(token)))
	assert.Nil(t, err)

	messageType, message, err := conn.ReadMessage()
	assert.Nil(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)

	if sendWrongToken {
		assert.Equal(t, "login>:"+"Not Authorized", string(message))
	} else {
		assert.Equal(t, "ok", string(message))
	}

	//test ping
	err = conn.WriteMessage(websocket.TextMessage, []byte("ping"))
	assert.Nil(t, err)

	messageType, message, err = conn.ReadMessage()

	assert.Nil(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "pong", string(message))

	conn.Close()
}

func publishTopic(topic string) {
	natsPool, err := NewPool(natsaddress, 2)
	busClient, err := natsPool.Get()
	if err != nil {
		log.Fatalf("Can't connect to nats: %v", err)
		return
	}

	// subscribe to nats
	err = busClient.Publish(topic, []byte("whosyourdaddy"))
	if err != nil {
		log.Fatalf("Can't connect to nats: %v", err)
		return
	}
}

func TestReceiveMessages(t *T) {
	server := startWsServer()

	NumberOfConnections := 1

	connectionsWaitGroup := sync.WaitGroup{}

	for i := 0; i < NumberOfConnections; i++ {
		connectionsWaitGroup.Add(1)
		go startTestReceiveConnection(t, &connectionsWaitGroup, MockUser)
	}

	connectionsWaitGroup.Wait()

	time.Sleep(10 * time.Second)

	server.Stop()
}

func startTestReceiveConnection(t *T, wg *sync.WaitGroup, userID string) {
	defer wg.Done()

	conn, _, err := dialer.Dial("ws://"+listeninterface+"/", nil)
	assert.Nil(t, err)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	token := []byte(MockJWT)

	//test ping
	err = conn.WriteMessage(websocket.TextMessage, []byte("ping"))
	assert.Nil(t, err)

	messageType, message, err := conn.ReadMessage()

	assert.Nil(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "pong", string(message))

	//test login
	err = conn.WriteMessage(websocket.TextMessage, []byte("login>:"+string(token)))
	assert.Nil(t, err)

	messageType, message, err = conn.ReadMessage()
	assert.Nil(t, err)
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "ok", string(message))

	// test topic
	// invalid topic
	err = conn.WriteMessage(websocket.TextMessage, []byte("topic>:"+string("paco.is.smart")))
	assert.Nil(t, err)

	messageType, message, err = conn.ReadMessage()
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "invalid topic", string(message))

	// valid topic
	// a mock publisher
	go publishTopic(natstopic)

	err = conn.WriteMessage(websocket.TextMessage, []byte("topic>:"+natstopic))
	assert.Nil(t, err)

	messageType, message, err = conn.ReadMessage()
	assert.Equal(t, websocket.TextMessage, messageType)
	assert.Equal(t, "whosyourdaddy", string(message))

	conn.Close()
}

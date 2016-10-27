package util

import (
	"time"
	"log"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 1 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 5 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Message is  a bare minimum representation of a websocket message.
type Message struct {
	Type int
	Data []byte
}

// WebSocketClient is an middleman between the websocket connection and the outside.
type WebSocketClient struct {
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of inbound messages.
	recv chan *Message

	// Buffered channel of outbound messages.
	send chan *Message
}

// NewWebSocketClient creates a new websocket client
func NewWebSocketClient(conn *websocket.Conn) *WebSocketClient {
	return &WebSocketClient{
		conn:   conn,
		recv:   make(chan *Message, 256),
		send:   make(chan *Message, 256),
	}
}

// readPump pumps messages from the websocket connection to the hub.
func (c *WebSocketClient) readPump() {
	defer func() {
		c.Close()
		close(c.recv)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		t, d, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("%v\n", err)
			/*
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				log.Printf("%v\n", err)
			}
			*/
			break
		}

		c.recv <- &Message{t, d}
	}
}

// write writes a message with the given message type and payload.
func (c *WebSocketClient) write(mt int, payload []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The hub closed the channel.
				c.write(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := c.conn.NextWriter(message.Type)
			if err != nil {
				return
			}
			w.Write(message.Data)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

// Run executes reader/writer routines without blocking
func (c *WebSocketClient) Run() {
	go func() {
		go c.writePump()
		c.readPump()
	}()
}

// Close closes underlying websocket connection
func (c *WebSocketClient) Close() error {
	return c.conn.Close()
}

// ReadMessage returns a Message reading channel
func (c *WebSocketClient) ReadMessage() <-chan *Message {
	return c.recv
}

// SendMessage enqueues a Message in the writing channel
func (c *WebSocketClient) SendMessage(m *Message) {
	c.send <- m
}


func (c *WebSocketClient) Conn() *websocket.Conn {
	return c.conn

}
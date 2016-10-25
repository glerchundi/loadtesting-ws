package util

import (
	"log"

	"gopkg.in/redis.v5"
)

type sendMessage struct {
	subj string
	msg  []byte
}

// PubSubClient is an middleman between the subscription and the outside.
type PubSubClient struct {
	// The pubsub connection.
	redisClient *redis.Client

	// The subscription.
	sub *redis.PubSub

	// closing flag
	closing bool

	// Buffered channel of inbound messages.
	recv chan []byte

	// Buffered channel of outbound messages.
	send chan *sendMessage
}

// NewPubSubClient creates a new pubsub client
func NewPubSubClient(redisClient *redis.Client) *PubSubClient {
	return &PubSubClient{
		redisClient: redisClient,
		closing:     false,
		recv:        make(chan []byte, 256),
		send:        make(chan *sendMessage, 256),
	}
}

func (c *PubSubClient) readPump() {
	defer func() {
		c.Close()
		close(c.recv)
	}()

	for {
		m, err := c.sub.ReceiveMessage()
		if err != nil && !c.closing {
			log.Printf("%v\n", err)
			break
		}

		if m != nil {
			c.recv <- []byte(m.Payload)
		}
	}
}

// writePump pumps messages from the hub to the pubsub connection.
func (c *PubSubClient) writePump() {
	defer func() {
		c.Close()
		close(c.send)
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// TODO: Loggear algo?
				return
			}

			err := c.redisClient.Publish(message.subj, string(message.msg)).Err()
			if err != nil {
				log.Printf("error: %v\n", err.Error())
				return
			}
		}
	}
}

// Run executes reader/writer routines without blocking
func (c *PubSubClient) Run(id string) error {
	// subscribe to redis channel
	sub, err := c.redisClient.Subscribe(id)
	if err != nil {
		return err
	}

	c.sub = sub
	go func() {
		go c.writePump()
		c.readPump()
	}()

	return nil
}

// Close closes underlying subscription
func (c *PubSubClient) Close() error {
	c.closing = true
	if c.sub != nil {
		return c.sub.Unsubscribe()
	}
	return nil
}

// ReadMessage returns a Message reading channel
func (c *PubSubClient) ReadMessage() <-chan []byte {
	return c.recv
}

// SendMessage enqueues a Message in the writing channel
func (c *PubSubClient) SendMessage(subj string, msg []byte) {
	c.send <- &sendMessage{subj, msg}
}

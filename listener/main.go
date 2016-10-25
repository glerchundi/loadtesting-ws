package main

import (
	"log"
	"net/http"
	"strings"
	"time"
	"strconv"

	"gopkg.in/redis.v5"
	"gopkg.in/tylerb/graceful.v1"
	"github.com/glerchundi/redis-issue/listener/util"
	common "github.com/glerchundi/redis-issue/util"
	"github.com/gorilla/websocket"
)

var (
	redisClient *redis.Client

	upgrader websocket.Upgrader
	waitGroup *common.WaitGroup
	quitting chan struct{}
)

type httpError struct {
	statusCode int
}

func (*httpError) Error() string {
	return ""
}

func websocketHandler(w http.ResponseWriter, r *http.Request) error {
	id := strings.TrimLeft(r.URL.Path, "/")

	var sleepMs time.Duration = -1
	if n, err := strconv.Atoi(strings.Split(id, "-")[0]); err != nil && n > 0 {
		sleepMs = time.Duration(n)
	}

	waitGroup.Add(1)
	defer waitGroup.Done()

	ttl := 10 * time.Second
	dlock := util.NewRedLock(redisClient, id, ttl)

	ok, err := dlock.Lock()
	if err != nil {
		return &httpError{http.StatusInternalServerError}
	}
	defer func() {
		if sleepMs > 0  {
			time.Sleep(sleepMs * time.Millisecond)
		}
		if err := dlock.Unlock(); err != nil {
			log.Printf("failed to unlock dlock for '%s': %v\n", id, err)
		}
	}()

	if !ok {
		return &httpError{http.StatusForbidden}
	}

	log.Printf("Client connected to: %s\n", r.URL)
	defer log.Printf("Client disconnected from: %s\n", r.URL)

	psclient := util.NewPubSubClient(redisClient)
	err = psclient.Run(id)
	if err != nil {
		return err
	}
	defer psclient.Close()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	wsclient := util.NewWebSocketClient(conn)
	wsclient.Run()
	defer wsclient.Close()

	ticker := time.NewTicker((ttl * 9) / 10)
	defer ticker.Stop()

	for {
		select {
		case <-quitting:
			return nil
		case <-ticker.C:
			if err := dlock.Renew(); err != nil {
				return err
			}
		case _, ok := <-psclient.ReadMessage():
			if !ok {
				return nil
			}
		case _, ok := <-wsclient.ReadMessage():
			if !ok {
				return nil
			}
		}


	}

	return nil
}

func main() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
		PoolSize: 5000,
	})

	_, err := redisClient.Ping().Result()
	if err != nil {
		log.Fatalf("%v", err)
	}

	upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	waitGroup = common.NewWaitGroup()
	quitting = make(chan struct{})

	log.Println("Listener started")
	defer log.Println("Listener stopped")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := websocketHandler(w, r); err != nil {
			var statusCode int = http.StatusInternalServerError
			if he, ok := err.(*httpError); ok {
				statusCode = he.statusCode
			}
			w.WriteHeader(statusCode)
		}
	})

	server := &graceful.Server{
		Timeout: 10 * time.Second,
		Server: &http.Server{
			Addr:    ":8080",
			Handler: mux,
		},
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("%v\n", err)
	}
	close(quitting)

	err = waitGroup.WaitTimeout(10 * time.Second)
	if err != nil {
		log.Fatalf("%v", err)
	}
}

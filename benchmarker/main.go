package main

import (
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
	"log"
	"bytes"
	"text/template"
	"math/rand"

	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
	common "github.com/glerchundi/redis-issue/util"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var url string = "ws://127.0.0.1:8080/{{ . }}-{{ randomString 32 }}"
	var origin string = ""
	var numConnections int = 1

	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	fs.StringVar(&url, "url", url, "")
	fs.StringVar(&origin, "origin", origin, "")
	fs.IntVar(&numConnections, "connections", numConnections, "")

	// parse
	fs.Parse(os.Args[1:])

	// graceful termination utilites
	waitGroup := common.NewWaitGroup()
	quitting := make(chan struct{})

	for i := 0; i < numConnections; i++ {
		go func(n int) {
			time.Sleep(time.Duration(n) * 10 * time.Millisecond)
			err := connectAndHandle(n, url, origin, waitGroup, quitting)
			if err != nil {
				log.Printf("%v\n", err)
			}
		}(i)
	}

	// start justin tunnel bench
	log.Println("Benchmarker started")
	defer log.Println("Benchmarker stopped")

	// wait for signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	L:
	for {
		select {
		case <-signalChan:
			break L
		}
	}

	// close existing websocket connections
	close(quitting)

	// block until
	err := waitGroup.WaitTimeout(60 * time.Second)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

func connectAndHandle(index int, templateURL, origin string, waitGroup *common.WaitGroup, quitting chan struct{}) error {
	waitGroup.Add(1)
	defer waitGroup.Done()

	endpointRaw, err := parseWithData(templateURL, &index)
	if err != nil {
		return err
	}
	endpoint := string(endpointRaw)

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	if origin == "" {
		originURL := *endpointURL
		if endpointURL.Scheme == "wss" {
			originURL.Scheme = "https"
		} else {
			originURL.Scheme = "http"
		}
		origin = originURL.String()
	}

	headers := make(http.Header)
	headers.Add("Origin", origin)

	log.Printf("Trying to connect to: %s\n", endpoint)

	conn, _, err := websocket.DefaultDialer.Dial(endpoint, headers)
	if err != nil {
		return err
	}
	ws := common.NewWebSocketClient(conn)

	log.Printf("Connected to: %s\n", endpoint)

	go func() {
		for {
			select {
			case _, ok := <-ws.ReadMessage():
				if !ok {
					log.Printf("websocket channel closed\n")
					return
				}
			}

			/*
			repMsg, err := handler(endpoint, string(reqMsg))
			if err != nil {
				return
			}

			if repMsg != "" {
				ws.WriteMessage(websocket.BinaryMessage, []byte(repMsg))
			}
			*/
		}
	}()

	L:
	for {
		select {
		case <-quitting:
			break L
		}
	}

	// send close message for graceful termination
	err = ws.Conn().WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		log.Printf("%v\n", err)
	}

	return ws.Close()
}

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	tmplFuncs = template.FuncMap{
		"randomString": func(n int) string {
			b := make([]rune, n)
			for i := range b {
				b[i] = letterRunes[rand.Intn(len(letterRunes))]
			}
			return string(b)
		},
	}
)

func parse(s string) ([]byte, error) {
	return parseWithData(s, nil)
}

func parseWithData(s string, data interface{}) ([]byte, error) {
	tmpl, err := template.New("test").Funcs(tmplFuncs).Parse(s)
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
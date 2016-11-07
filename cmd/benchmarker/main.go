package main

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/glerchundi/loadtesting-ws/util"
	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
	"gopkg.in/tylerb/graceful.v1"
)

const (
	cliName = "benchmarker"
)

var (
	Version = "0.0.0"
	GitRev  = "----------------------------------------"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var port int = 8080
	var origin string = ""
	var connections int = 1
	var concurrency int = 1

	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	fs.IntVar(&port, "port", port, "")
	fs.StringVar(&origin, "origin", origin, "")
	fs.IntVar(&concurrency, "concurrency", concurrency, "")
	fs.IntVar(&connections, "connections", connections, "")

	// set normalization func
	fs.SetNormalizeFunc(
		func(f *pflag.FlagSet, name string) pflag.NormalizedName {
			if strings.Contains(name, "_") {
				return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
			}
			return pflag.NormalizedName(name)
		},
	)

	// define usage
	fs.Usage = func() {
		goVersion := strings.TrimPrefix(runtime.Version(), "go")
		fmt.Fprintf(os.Stderr, "%s (version=%s, gitrev=%s, go=%s)\n", cliName, Version, GitRev, goVersion)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fs.PrintDefaults()
	}

	// parse
	fs.Parse(os.Args[1:])
	if len(fs.Args()) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	// get url
	url := fs.Arg(0)

	// graceful termination utilites
	waitGroup := util.NewWaitGroup()
	quitting := make(chan struct{})
	connsCh := make(chan int)

	// create main creator loop
	go func(url, origin string, concurrency int, waitGroup *util.WaitGroup, quitting chan struct{}) {
		for {
			select {
			case <-quitting:
				return
			case connections, ok := <-connsCh:
				if !ok {
					return
				}

				createConnections(url, origin, concurrency, connections, waitGroup, quitting)
			}
		}
	}(url, origin, concurrency, waitGroup, quitting)

	// start justin tunnel bench
	log.Println("Benchmarker started")
	defer log.Println("Benchmarker stopped")

	// create first connections
	connsCh <- connections

	mux := http.NewServeMux()
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	})
	mux.HandleFunc("/inc", func(w http.ResponseWriter, r *http.Request) {
		if n, err := strconv.Atoi(r.URL.Query().Get("count")); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte{})
		} else {
			connsCh <- n
		}
	})

	server := &graceful.Server{
		Timeout: 10 * time.Second,
		Server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("%v\n", err)
	}

	// close existing websocket connections
	close(quitting)

	// block until
	err := waitGroup.WaitTimeout(60 * time.Second)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

func createConnections(url, origin string, concurrency, connections int, waitGroup *util.WaitGroup, quitting chan struct{}) {
	connsPerRoutine := int(connections / concurrency)
	for i := 0; i < concurrency; i++ {
		currentConnsPerRoutine := connsPerRoutine
		if i == concurrency-1 {
			currentConnsPerRoutine = connections - i*connsPerRoutine
		}

		go func(i, count, countPerRoutine int) {
			for j := 0; j < count; j++ {
				err := connectAndHandle(i*countPerRoutine+j, url, origin, waitGroup, quitting)
				if err != nil {
					log.Printf("%v\n", err)
				}
			}
		}(i, currentConnsPerRoutine, connsPerRoutine)
	}
}

func connectAndHandle(index int, templateURL, origin string, waitGroup *util.WaitGroup, quitting chan struct{}) error {
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

	ws := util.NewWebSocketClient(conn)
	ws.Run()

	log.Printf("Connected to: %s\n", endpoint)

	go func() {
		defer log.Printf("Disconnected from: %s\n", endpoint)

		waitGroup.Add(1)
		defer waitGroup.Done()

		ticker := time.NewTimer(500 * time.Millisecond)
		defer ticker.Stop()

		isQuitting := false
		for {
		L:
			select {
			case _, ok := <-ws.ReadMessage():
				if !ok {
					return
				}
			case <-ticker.C:
				select {
				case <-quitting:
					if isQuitting {
						break L
					}

					// send close message for graceful termination
					ws.SendMessage(&util.Message{
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, ""),
					})
					isQuitting = true
				}
			}
		}
	}()

	return nil
}

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	tmplFuncs   = template.FuncMap{
		"randomString": func(n int) string {
			b := make([]rune, n)
			for i := range b {
				b[i] = letterRunes[rand.Intn(len(letterRunes))]
			}
			return string(b)
		},
	}
)

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

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/glerchundi/loadtesting-ws/util"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"
	"gopkg.in/tylerb/graceful.v1"
)

const (
	cliName = "listener"
)

var (
	Version = "0.0.0"
	GitRev  = "----------------------------------------"
)

var (
	upgrader  websocket.Upgrader
	waitGroup *util.WaitGroup
	quitting  chan struct{}

	wsConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_connections_active",
			Help: "Total number of connections.",
		},
	)

	wsConnectionsFailed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ws_connections_failed",
			Help: "Failed number of connections.",
		},
	)
)

type httpError struct {
	code int
}

func (he *httpError) Error() string {
	return fmt.Sprintf("http error: %d", he.code)
}

func init() {
	prometheus.MustRegister(wsConnectionsActive)
	prometheus.MustRegister(wsConnectionsFailed)
}

func main() {
	var port int = 8080

	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	fs.IntVar(&port, "port", port, "")

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

	upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	waitGroup = util.NewWaitGroup()
	quitting = make(chan struct{})

	log.Println("Listener started")
	defer log.Println("Listener stopped")

	mux := http.NewServeMux()
	mux.Handle("/metrics", prometheus.Handler())
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		if err := websocketHandler(w, r); err != nil {
			wsConnectionsFailed.Inc()

			log.Printf("%v\n", err)

			if he, ok := err.(*httpError); ok {
				w.WriteHeader(he.code)
				w.Write([]byte{})
			}

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

	close(quitting)

	err := waitGroup.WaitTimeout(10 * time.Second)
	if err != nil {
		log.Fatalf("%v", err)
	}
}

func websocketHandler(w http.ResponseWriter, r *http.Request) error {
	waitGroup.Add(1)
	defer waitGroup.Done()

	wsConnectionsActive.Inc()
	defer wsConnectionsActive.Dec()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return &httpError{http.StatusForbidden}
	}

	wsclient := util.NewWebSocketClient(conn)
	wsclient.Run()
	defer wsclient.Close()

	log.Printf("Client connected to: %s\n", r.URL)
	defer log.Printf("Client disconnected from: %s\n", r.URL)

	for {
		select {
		case <-quitting:
			return nil
		case _, ok := <-wsclient.ReadMessage():
			if !ok {
				return nil
			}
		}
	}
}

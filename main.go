package main

import (
	"encoding/json"
	"log"
	"net/http"
	"nubewatch/api/proto/protoserver"
	"nubewatch/cluster/consensus"
	"nubewatch/discover"
	"nubewatch/internal/config"
	"runtime"
	"sync"
	"time"
)

func main() {
	var wg sync.WaitGroup
	if runtime.GOOS == "windows" {
		log.Fatalln("nubewatch is only compatible with Mac and Linux")
	}

	n := consensus.New()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[proto] Starting proto server...")
		protoserver.Start(n)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		discover.ServeAndBlock(n.Host, config.DiscoverPort)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		healthcheckEndpoint(n)
	}()

	wg.Wait()
}

func healthcheckEndpoint(n *consensus.Node) {
	const timeout = 10 * time.Second
	http.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !n.IsHealthy() {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// GOSEC: GO-S2114
	srv := &http.Server{
		Addr:              ":80",
		ReadHeaderTimeout: timeout,
	}
	err := srv.ListenAndServe()
	if err != nil {
		log.Fatalln(err)
	}
}

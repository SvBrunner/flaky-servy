package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	configstore "github.com/SvBrunner/flaky-servy/internal/config"
	apihttp "github.com/SvBrunner/flaky-servy/internal/http"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP listen address")
		configDir = flag.String("config-dir", "./configs", "directory containing YAML config files")
	)
	flag.Parse()

	store := configstore.NewStore(*configDir)
	handler := apihttp.NewHandler(store)

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("serving configs from %q on %s", *configDir, *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

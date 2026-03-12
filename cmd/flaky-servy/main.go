package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
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

	issuer := requiredEnv("OIDC_ISSUER")
	audience := requiredEnv("OIDC_AUDIENCE")
	clientID := requiredEnv("OIDC_CLIENT_ID")
	clientSecret := requiredEnv("OIDC_CLIENT_SECRET")
	redirectURL := os.Getenv("OIDC_REDIRECT_URI")
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/oidc/callback"
	}

	oidcAuth, err := apihttp.NewOIDCAuth(context.Background(), issuer, audience, clientID, clientSecret, redirectURL)
	if err != nil {
		log.Fatalf("failed to initialize oidc auth: %v", err)
	}

	store := configstore.NewStore(*configDir)
	handler := apihttp.NewHandlerWithOIDC(store, oidcAuth)

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

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("missing required environment variable %s", name)
	}
	return value
}

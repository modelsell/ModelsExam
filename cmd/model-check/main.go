package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"model-check/internal/httpgzip"
	"model-check/internal/server"
	"model-check/internal/store"
	"model-check/web"
)

func main() {
	st, err := store.Open(os.Getenv("SQL_DSN"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	var webFS fs.FS
	if sub, err := web.FS(); err == nil {
		webFS = sub
	} else {
		log.Printf("frontend not embedded (%v); serving API only", err)
	}
	var trusted []string
	if v := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); v != "" {
		trusted = strings.Split(v, ",")
	}
	srv := server.New(server.Config{
		Store:         st,
		AllowPrivate:  os.Getenv("MODEL_CHECK_ALLOW_PRIVATE") == "true",
		Web:           webFS,
		Trusted:       trusted,
		VerifyBaseURL: os.Getenv("MODEL_CHECK_VERIFY_BASE_URL"),
		SiteURL:       os.Getenv("MODEL_CHECK_SITE_URL"),
	})
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	httpServer := &http.Server{Addr: addr, Handler: httpgzip.Handler(srv.Handler()), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	log.Printf("ModelsExam listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

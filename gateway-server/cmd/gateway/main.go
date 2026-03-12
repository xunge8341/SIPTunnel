package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/server"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	port := readPort()
	paths := config.LoadStoragePathsFromEnv()
	if err := paths.EnsureWritable(); err != nil {
		log.Fatalf("startup directory validation failed: %v", err)
	}
	handler, closer, err := server.NewHandlerWithOptions(server.HandlerOptions{LogDir: paths.LogDir, AuditDir: paths.AuditDir})
	if err != nil {
		log.Fatalf("initialize handler failed: %v", err)
	}
	if closer != nil {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Printf("close resources failed: %v", err)
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("gateway server listening on %s (version=%s commit=%s build_time=%s)", httpServer.Addr, version, commit, buildTime)
		log.Printf("storage paths temp_dir=%q final_dir=%q audit_dir=%q log_dir=%q", paths.TempDir, paths.FinalDir, paths.AuditDir, paths.LogDir)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func readPort() string {
	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		return "18080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Printf("invalid GATEWAY_PORT=%q, fallback to 18080", port)
		return "18080"
	}
	return port
}

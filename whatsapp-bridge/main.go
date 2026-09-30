package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	_ "github.com/mattn/go-sqlite3"
)

func initSentry() {
	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN == "" {
		return
	}

	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = "production"
	}
	release := os.Getenv("SENTRY_RELEASE")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		Environment:      env,
		Release:          release,
		TracesSampleRate: 0.1,
	})
	if err != nil {
		fmt.Printf("Warning: failed to initialize Sentry: %v\n", err)
	} else {
		fmt.Println("Sentry initialized for WhatsApp Bridge.")
	}
}

func main() {
	initSentry()
	defer sentry.Flush(2 * time.Second)

	defaultPort := 8080
	if envPort := os.Getenv("WHATSAPP_BRIDGE_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			defaultPort = p
		}
	}

	sessionDBPath := flag.String("session-db", "whatsapp_session.db", "Path to SQLite session database")
	storageDBPath := flag.String("storage-db", "whatsapp_data.db", "Path to SQLite chats/messages/contacts database")
	port := flag.Int("port", defaultPort, "Port to bind local-only HTTP/RPC server (127.0.0.1)")
	logLevel := flag.String("log-level", "INFO", "Logging level (DEBUG, INFO, WARN, ERROR)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Printf("Starting WhatsApp Bridge on 127.0.0.1:%d...\n", *port)
	bridge, err := NewWhatsAppBridge(ctx, *sessionDBPath, *storageDBPath, *logLevel)
	if err != nil {
		sentry.CaptureException(err)
		fmt.Printf("Fatal: failed to initialize WhatsApp bridge: %v\n", err)
		os.Exit(1)
	}

	httpServer := NewHTTPServer(bridge, *port)
	go func() {
		if err := httpServer.Start(); err != nil && err != http.ErrServerClosed {
			sentry.CaptureException(err)
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	err = bridge.Connect(ctx)
	if err != nil {
		sentry.CaptureException(err)
		fmt.Printf("Warning: initial connect returned: %v\n", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down WhatsApp bridge gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)

	bridge.Disconnect()
	fmt.Println("Bridge stopped.")
}

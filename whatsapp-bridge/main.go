package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	sessionDBPath := flag.String("session-db", "whatsapp_session.db", "Path to SQLite session database")
	storageDBPath := flag.String("storage-db", "whatsapp_data.db", "Path to SQLite chats/messages/contacts database")
	logLevel := flag.String("log-level", "INFO", "Logging level (DEBUG, INFO, WARN, ERROR)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Starting WhatsApp Bridge...")
	bridge, err := NewWhatsAppBridge(ctx, *sessionDBPath, *storageDBPath, *logLevel)
	if err != nil {
		fmt.Printf("Fatal: failed to initialize WhatsApp bridge: %v\n", err)
		os.Exit(1)
	}

	err = bridge.Connect(ctx)
	if err != nil {
		fmt.Printf("Warning: initial connect returned: %v\n", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down WhatsApp bridge gracefully...")
	bridge.Disconnect()
	fmt.Println("Bridge stopped.")
}

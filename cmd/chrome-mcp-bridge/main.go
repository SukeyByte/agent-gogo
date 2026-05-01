package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/SukeyByte/agent-gogo/internal/provider/chromemcpbridge"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9222", "HTTP bridge address")
	debugPort := flag.Int("debug-port", 9223, "Chrome DevTools remote debugging port")
	chromePath := flag.String("chrome-path", "", "Chrome executable path")
	userDataDir := flag.String("user-data-dir", "", "Chrome profile directory")
	headless := flag.Bool("headless", false, "run Chrome in headless mode")
	maxSummaryLength := flag.Int("max-summary-length", 12000, "maximum extracted page text length")
	flag.Parse()

	bridge := chromemcpbridge.New(chromemcpbridge.Config{
		DebugPort:        *debugPort,
		ChromePath:       *chromePath,
		UserDataDir:      *userDataDir,
		Headless:         *headless,
		MaxSummaryLength: *maxSummaryLength,
	})
	defer bridge.Close()

	server := &http.Server{
		Addr:    *addr,
		Handler: bridge.Handler(),
	}
	go func() {
		log.Printf("chrome mcp bridge listening on http://%s", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	fmt.Println("shutting down chrome mcp bridge")
	_ = server.Shutdown(context.Background())
}

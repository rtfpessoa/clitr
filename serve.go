package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rtfpessoa/clitr/internal/client"
	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/web"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	portFlagKeyLong  = "port"
	portFlagKeyShort = "P"

	hostFlagKeyLong = "host"

	trustedProxyFlagKeyLong = "trusted-proxy"
)

type serveConfig struct {
	*rootConfig

	port       int
	host       string
	trustProxy bool
}

func NewServeCmd(rootConfig *rootConfig) *cobra.Command {
	serveCfg := &serveConfig{
		rootConfig: rootConfig,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start web server for transaction CSV export",
		Long: `Start a local web server that provides a browser-based interface
for authenticating with Trade Republic, fetching transactions,
and copying the CSV export to clipboard.

All credentials and transaction data are processed in memory only
and never written to disk or logs.`,
	}

	serveCmd.Flags().IntVarP(&serveCfg.port, portFlagKeyLong, portFlagKeyShort, 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&serveCfg.host, hostFlagKeyLong, "0.0.0.0", "Host to bind to")
	serveCmd.Flags().BoolVar(&serveCfg.trustProxy, trustedProxyFlagKeyLong, false, "Trust X-Forwarded-For header from reverse proxy for rate limiting")

	serveCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runServe(serveCfg)
	}

	return serveCmd
}

func runServe(config *serveConfig) error {
	cfg := web.ServerConfig{
		Host:       config.host,
		Port:       config.port,
		TrustProxy: config.trustProxy,
	}

	// Client factory creates a new TR client for each login attempt.
	// The web server does not save cookies — credentials are in-memory only.
	clientFactory := func(phone string) (web.WebClient, error) {
		c, err := client.NewClient(phone, "", false)
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}
		return c, nil
	}

	srv, err := web.NewServer(cfg, clientFactory)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	case sig := <-sigCh:
		log.Info("Received shutdown signal", zap.String("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		return nil
	}
}

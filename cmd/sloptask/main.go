// @title			SlopTask API
// @version		1.0
// @description	Task tracker for coordinating AI agents with deadlines and state machine.
// @BasePath		/api/v1

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mtlprog/sloptask/internal/config"
	"github.com/mtlprog/sloptask/internal/database"
	"github.com/mtlprog/sloptask/internal/handler"
	"github.com/mtlprog/sloptask/internal/logger"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "sloptask",
		Usage: "Task tracker for AI agents",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				Value:   "info",
				Usage:   "Log level (debug, info, warn, error)",
				EnvVars: []string{"LOG_LEVEL"},
			},
			&cli.StringFlag{
				Name:     "database-url",
				Aliases:  []string{"d"},
				Value:    config.DefaultDatabaseURL,
				Usage:    "PostgreSQL database URL",
				EnvVars:  []string{"DATABASE_URL"},
				Required: true,
			},
		},
		Before: func(c *cli.Context) error {
			logger.Setup(logger.ParseLevel(c.String("log-level")))
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Start the web server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Value:   config.DefaultPort,
						Usage:   "HTTP server port",
						EnvVars: []string{"PORT"},
					},
				},
				Action: runServe,
			},
			{
				Name:   "check-deadlines",
				Usage:  "Check and update expired task deadlines",
				Action: runCheckDeadlines,
			},
		},
		Action: runServe,
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("application error", "error", err)
		os.Exit(1)
	}
}

func runServe(c *cli.Context) error {
	ctx := c.Context

	port := c.String("port")
	if port == "" {
		port = config.DefaultPort
	}
	databaseURL := c.String("database-url")

	db, err := database.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, db.Pool()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	h := handler.New(db.Pool())

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("starting server", "server_addr", "http://localhost:"+port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-done:
		slog.Info("shutting down server")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

func runCheckDeadlines(c *cli.Context) error {
	ctx := c.Context
	databaseURL := c.String("database-url")

	db, err := database.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, db.Pool()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("deadline checker stub - not implemented yet")
	slog.Info("this command will check and update expired task deadlines in future iterations")

	return nil
}

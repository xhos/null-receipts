package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	ollamaapi "github.com/ollama/ollama/api"
	"google.golang.org/genai"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"null-receipts/internal/config"
	nullv1 "null-receipts/internal/gen/null/v1"
	"null-receipts/internal/server"
)

func main() {
	cfg := config.Load()

	var formatter log.Formatter
	if cfg.LogFormat == "json" {
		formatter = log.JSONFormatter
	} else {
		formatter = log.TextFormatter
	}

	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		Level:           cfg.LogLevel,
		Formatter:       formatter,
	})

	var ocrService *server.Server

	switch cfg.Provider {
	case "gemini":
		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:  cfg.GeminiAPIKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			logger.Fatal("failed to create gemini client", "err", err)
		}
		ocrService = server.New(nil, client, cfg.GeminiModel, logger)

	default:
		client, err := ollamaapi.ClientFromEnvironment()
		if err != nil {
			logger.Fatal("failed to create ollama client", "err", err)
		}
		ocrService = server.New(client, nil, cfg.OllamaModel, logger)
	}

	lis, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		logger.Fatal("failed to listen", "err", err)
	}

	srv := grpc.NewServer()
	nullv1.RegisterReceiptOCRServiceServer(srv, ocrService)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)

	reflection.Register(srv)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down...")
		healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		srv.GracefulStop()
	}()

	logger.Info("server started", "addr", cfg.ListenAddress, "provider", cfg.Provider)
	if err := srv.Serve(lis); err != nil {
		logger.Fatal("serve failed", "err", err)
	}
}

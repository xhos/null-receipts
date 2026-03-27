package main

import (
	"context"
	"io"
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

	logWriter := io.Writer(os.Stderr)
	logFormatter := log.TextFormatter

	if cfg.LogFormat != "text" {
		logFile, err := os.OpenFile("null-receipts.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("failed to create log file", "err", err)
		}
		defer logFile.Close()

		logWriter = io.MultiWriter(os.Stderr, logFile)
		logFormatter = log.JSONFormatter
	}

	logger := log.NewWithOptions(logWriter, log.Options{
		ReportTimestamp: true,
		Level:           cfg.LogLevel,
		Formatter:       logFormatter,
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

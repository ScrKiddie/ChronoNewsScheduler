package main

import (
	"chrononews-scheduler/internal/adapter"
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/service"
	"chrononews-scheduler/internal/service/compression"
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/robfig/cron/v3"
)

func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func runServices(appCfg *config.Config, jobCtx context.Context, storage *adapter.StorageAdapter) {
	mode := strings.ToLower(appCfg.AppMode)
	slog.Info("Cron job terpicu.", "mode", mode)

	runAll := mode == "all"

	if runAll || mode == "janitor" {
		slog.Info("Memulai service: Janitor")
		service.RunJanitorScheduler(appCfg.JanitorStuckThreshold)
		slog.Info("Service Janitor selesai.")
	}

	if runAll || mode == "compression" {
		slog.Info("Memulai service: Compression")
		compression.RunScheduler(jobCtx, appCfg, storage)
		slog.Info("Service Compression selesai.")
	}

	if runAll || mode == "deletion" {
		slog.Info("Memulai service: Deletion Queue")
		service.ProcessDeletionQueue(
			appCfg.DeletionQueueBatchSize,
			appCfg.DeletionQueueMaxRetries,
			storage,
		)
		slog.Info("Service Deletion Queue selesai.")
	}

	if runAll || mode == "cleanup" {
		slog.Info("Memulai service: Cleanup Orphaned Files")
		service.CleanupOrphanedFiles(
			appCfg,
			appCfg.CleanupBatchSize,
			storage,
		)
		slog.Info("Service Cleanup Orphaned Files selesai.")
	}
	slog.Info("Semua service selesai.")
}

func main() {
	appCfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Konfigurasi tidak valid: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(appCfg.LogLevel),
	}))
	slog.SetDefault(logger)

	database.ConnectDB(appCfg.DSN)

	slog.Info("Aplikasi dimulai",
		slog.String("schedule", appCfg.AppSchedule),
		slog.String("mode", appCfg.AppMode),
		slog.String("storage", appCfg.StorageMode),
	)

	var s3Client *s3.Client
	if appCfg.StorageMode == "s3" {
		slog.Info("Menginisialisasi AWS S3 Client...")

		cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
			awsConfig.WithRegion(appCfg.S3Region),
			awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				appCfg.S3AccessKey,
				appCfg.S3SecretKey,
				"",
			)),
		)
		if err != nil {
			log.Fatalf("Gagal load config AWS: %v", err)
		}

		if appCfg.S3Endpoint != "" {
			s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
				o.BaseEndpoint = aws.String(appCfg.S3Endpoint)
				o.UsePathStyle = true
				o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
				o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
			})
		} else {
			s3Client = s3.NewFromConfig(cfg)
		}

	}

	storageAdapter := adapter.NewStorageAdapter(appCfg, s3Client)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := cron.New()
	_, err = c.AddFunc(appCfg.AppSchedule, func() {
		jobCtx, jobCancel := context.WithTimeout(ctx, 30*time.Minute)
		defer jobCancel()
		runServices(appCfg, jobCtx, storageAdapter)
	})
	if err != nil {
		slog.Error("Gagal menambahkan cron job", "error", err)
		os.Exit(1)
	}

	c.Start()
	slog.Info("Scheduler berjalan. Tekan Ctrl+C untuk berhenti.")
	<-ctx.Done()

	slog.Info("Sinyal berhenti diterima, menghentikan scheduler...")
	c.Stop()
	slog.Info("Scheduler berhenti.")
}

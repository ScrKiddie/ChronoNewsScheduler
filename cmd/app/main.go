package main

import (
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/service"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

func main() {
	appCfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Konfigurasi tidak valid", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(appCfg.LogLevel),
	}))
	slog.SetDefault(logger)

	database.ConnectDB(appCfg.DSN)

	schedulerCfg := service.SchedulerConfig{
		BatchSize:     appCfg.BatchSize,
		SourceDir:     appCfg.SourceDir,
		DestDir:       appCfg.DestDir,
		IsConcurrent:  appCfg.IsConcurrent,
		IsTestMode:    appCfg.IsTestMode,
		NumIOWorkers:  appCfg.NumIOWorkers,
		NumCPUWorkers: appCfg.NumCPUWorkers,
		WebPQuality:   appCfg.WebPQuality,
		MaxWidth:      appCfg.MaxWidth,
		MaxHeight:     appCfg.MaxHeight,
		MaxRetries:    appCfg.MaxRetries,
	}

	slog.Info("Aplikasi dimulai dengan konfigurasi dari environment variables",
		slog.Group("scheduler",
			slog.Bool("concurrent", schedulerCfg.IsConcurrent),
			slog.Bool("test_mode", schedulerCfg.IsTestMode),
			slog.Int("batch_size", schedulerCfg.BatchSize),
			slog.Int("io_workers", schedulerCfg.NumIOWorkers),
			slog.Int("cpu_workers", schedulerCfg.NumCPUWorkers),
			slog.Int("max_retries", schedulerCfg.MaxRetries),
			slog.String("source_dir", schedulerCfg.SourceDir),
			slog.String("destination_dir", schedulerCfg.DestDir),
		),
		slog.Group("image_processing",
			slog.Int("webp_quality", schedulerCfg.WebPQuality),
			slog.Int("max_width", schedulerCfg.MaxWidth),
			slog.Int("max_height", schedulerCfg.MaxHeight),
		),
		slog.String("schedule", appCfg.Schedule),
		slog.String("log_level", appCfg.LogLevel),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if appCfg.Schedule != "" {
		slog.Info("Memulai dalam mode terjadwal", "schedule", appCfg.Schedule)
		c := cron.New()
		_, err := c.AddFunc(appCfg.Schedule, func() {
			slog.Info("Cron job terpicu, menjalankan tugas kompresi.")
			jobCtx, jobCancel := context.WithCancel(ctx)
			defer jobCancel()
			service.RunScheduler(jobCtx, schedulerCfg)
		})
		if err != nil {
			slog.Error("Tidak dapat menambahkan cron job", "error", err)
			os.Exit(1)
		}
		c.Start()

		slog.Info("Scheduler berjalan. Tekan Ctrl+C untuk berhenti.")
		<-ctx.Done()

		slog.Info("Sinyal berhenti diterima, menghentikan scheduler...")
		c.Stop()
		slog.Info("Scheduler berhenti.")
	} else {
		slog.Info("Memulai dalam mode eksekusi tunggal.")
		service.RunScheduler(ctx, schedulerCfg)
	}
}

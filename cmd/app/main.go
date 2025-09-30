package main

import (
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/service"
	"chrononews-scheduler/internal/service/compression"
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

	schedulerCfg := compression.SchedulerConfig{
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
		slog.Group("schedules",
			slog.String("compression", appCfg.CompressionSchedule),
			slog.String("cleanup_unused", appCfg.CleanupSchedule),
			slog.String("janitor_stuck_tasks", appCfg.JanitorSchedule),
			slog.String("deletion_queue_source_files", appCfg.DeletionQueueSchedule),
		),
		slog.String("log_level", appCfg.LogLevel),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if appCfg.CompressionSchedule != "" || appCfg.CleanupSchedule != "" || appCfg.JanitorSchedule != "" || appCfg.DeletionQueueSchedule != "" {
		c := cron.New()

		if appCfg.CompressionSchedule != "" {
			slog.Info("Menjadwalkan Compression runner", "schedule", appCfg.CompressionSchedule)
			_, err := c.AddFunc(appCfg.CompressionSchedule, func() {
				slog.Info("Cron job kompresi terpicu.")
				jobCtx, jobCancel := context.WithCancel(ctx)
				defer jobCancel()
				compression.RunScheduler(jobCtx, schedulerCfg)
			})
			if err != nil {
				slog.Error("Tidak dapat menambahkan cron job kompresi", "error", err)
				os.Exit(1)
			}
		}

		if appCfg.CleanupSchedule != "" {
			slog.Info("Menjadwalkan Cleanup runner untuk file yang lama tidak terpakai", "schedule", appCfg.CleanupSchedule)
			_, err := c.AddFunc(appCfg.CleanupSchedule, func() {
				slog.Info("Cron job cleanup unused file terpicu.")
				service.RunCleanupOldUnusedFiles(
					appCfg.DestDir,
					appCfg.CleanupThreshold,
					appCfg.CleanupBatchSize,
				)
			})
			if err != nil {
				slog.Error("Tidak dapat menambahkan cron job cleanup", "error", err)
				os.Exit(1)
			}
		}

		if appCfg.JanitorSchedule != "" {
			slog.Info("Menjadwalkan Janitor runner", "schedule", appCfg.JanitorSchedule)
			_, err := c.AddFunc(appCfg.JanitorSchedule, func() {
				slog.Info("Cron job janitor terpicu.")
				service.RunJanitorScheduler(appCfg.JanitorStuckThreshold)
			})
			if err != nil {
				slog.Error("Tidak dapat menambahkan cron job janitor", "error", err)
				os.Exit(1)
			}
		}

		if appCfg.DeletionQueueSchedule != "" {
			slog.Info("Menjadwalkan proses antrean penghapusan file sumber", "schedule", appCfg.DeletionQueueSchedule)
			_, err := c.AddFunc(appCfg.DeletionQueueSchedule, func() {
				slog.Info("Cron job antrean penghapusan file sumber terpicu.")
				service.ProcessSourceFileDeletionQueue(
					appCfg.DeletionQueueBatchSize,
					appCfg.DeletionQueueMaxRetries,
				)
			})
			if err != nil {
				slog.Error("Tidak dapat menambahkan cron job antrean penghapusan", "error", err)
				os.Exit(1)
			}
		}

		c.Start()
		slog.Info("Scheduler berjalan. Tekan Ctrl+C untuk berhenti.")
		<-ctx.Done()

		slog.Info("Sinyal berhenti diterima, menghentikan scheduler...")
		c.Stop()
		slog.Info("Scheduler berhenti.")
	} else {
		slog.Info("Tidak ada jadwal yang dikonfigurasi. Aplikasi akan keluar.")
	}
}

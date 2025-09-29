package compression

import (
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SchedulerConfig struct {
	BatchSize     int
	SourceDir     string
	DestDir       string
	IsConcurrent  bool
	IsTestMode    bool
	NumIOWorkers  int
	NumCPUWorkers int
	WebPQuality   int
	MaxWidth      int
	MaxHeight     int
	MaxRetries    int
}

func monitorPeakRAM(p *process.Process, done <-chan struct{}) (peakRAM uint64) {
	var currentPeakRAM uint64
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			peakRAM = currentPeakRAM
			return
		case <-ticker.C:
			memInfo, err := p.MemoryInfo()
			if err == nil {
				if memInfo.RSS > currentPeakRAM {
					currentPeakRAM = memInfo.RSS
				}
			}
		}
	}
}
func logResourceUsage(duration time.Duration, cpuTimeBefore, cpuTimeAfter float64, peakRAM uint64) {
	cpuTimeUsed := cpuTimeAfter - cpuTimeBefore
	cpuPercent := 0.0
	if duration.Seconds() > 0 {
		cpuPercent = (cpuTimeUsed / duration.Seconds()) * 100.0
	}

	slog.Info("Metrik Kinerja Proses Selesai",
		"total_duration", duration.String(),
		"cpu_utilization_percent", fmt.Sprintf("%.2f%%", cpuPercent),
		"peak_ram_mb", fmt.Sprintf("%.2f MB", float64(peakRAM)/1024/1024),
	)
}

func RunScheduler(ctx context.Context, cfg SchedulerConfig) {
	slog.Info("Scheduler dimulai.")
	mode := "Sekuensial"
	if cfg.IsConcurrent {
		mode = "Konkuren (Pipeline)"
	}
	slog.Info("Detail eksekusi", "mode", mode, "batch_size", cfg.BatchSize)

	tasks := getTasksFromDB(ctx, cfg)
	if len(tasks) == 0 {
		slog.Info("Tidak ada tugas kompresi yang tertunda.")
		return
	}

	logMessage := fmt.Sprintf("Menemukan %d tugas untuk diproses.", len(tasks))
	if !cfg.IsTestMode {
		logMessage = fmt.Sprintf("Menemukan dan mengunci %d tugas untuk diproses.", len(tasks))
	}
	slog.Info(logMessage)

	startTime := time.Now()

	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		slog.Warn("Gagal mendapatkan info proses untuk metrik", "error", err)
	}
	cpuTimesBefore, _ := p.Times()
	cpuTimeBefore := cpuTimesBefore.User + cpuTimesBefore.System

	doneMonitoring := make(chan struct{})
	var peakRAM uint64
	go func() {
		peakRAM = monitorPeakRAM(p, doneMonitoring)
	}()

	if cfg.IsConcurrent {
		runConcurrentPipeline(ctx, tasks, cfg)
	} else {
		runSequential(ctx, tasks, cfg)
	}

	close(doneMonitoring)
	duration := time.Since(startTime)
	cpuTimesAfter, _ := p.Times()
	cpuTimeAfter := cpuTimesAfter.User + cpuTimesAfter.System

	logResourceUsage(duration, cpuTimeBefore, cpuTimeAfter, peakRAM)
	slog.Info("Scheduler selesai.")
}

func getTasksFromDB(ctx context.Context, cfg SchedulerConfig) []model.File {
	var tasks []model.File

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			WithContext(ctx).
			Where("status = ? AND failed_attempts < ?", "pending", cfg.MaxRetries).
			Limit(cfg.BatchSize).
			Find(&tasks).Error

		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		if cfg.IsTestMode {
			slog.Debug("Mode Tes: Melewati penguncian status tugas di database.")
			return nil
		}

		var taskIDs []int32
		for _, task := range tasks {
			taskIDs = append(taskIDs, task.ID)
		}

		return tx.Model(&model.File{}).Where("id IN ?", taskIDs).Update("status", "processing").Error
	})

	if err != nil {
		slog.Error("Gagal mengambil tugas dari database", "error", err)
		return nil
	}
	return tasks
}

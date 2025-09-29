package service

import (
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"log/slog"
	"time"
)

func RunJanitorScheduler(threshold time.Duration) {
	slog.Info("Memulai scheduler janitor...")

	stuckTime := time.Now().Add(-threshold)
	unixThreshold := stuckTime.Unix()

	result := database.DB.Model(&model.File{}).
		Where("status = ? AND updated_at < ?", "processing", unixThreshold).
		Update("status", "pending")

	if result.Error != nil {
		slog.Error("Scheduler janitor gagal saat query database", "error", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		slog.Warn("Janitor: Mereset tugas yang macet", "jumlah", result.RowsAffected)
	} else {
		slog.Info("Janitor: Tidak ada tugas yang macet ditemukan.")
	}

	slog.Info("Scheduler janitor selesai.")
}

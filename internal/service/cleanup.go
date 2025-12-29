package service

import (
	"chrononews-scheduler/internal/adapter"
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/constant"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

func CleanupOrphanedFiles(cfg *config.Config, batchSize int, storage *adapter.StorageAdapter) {
	slog.Info("Memulai tugas pembersihan orphaned file...")

	thresholdTime := time.Now().Add(-cfg.CleanupThreshold)
	unixThreshold := thresholdTime.Unix()

	var orphanedFiles []model.File
	err := database.DB.Where("used_by_post_id IS NULL AND used_by_user_id IS NULL AND created_at < ?", unixThreshold).
		Limit(batchSize).
		Find(&orphanedFiles).Error

	if err != nil {
		slog.Error("Gagal mengambil data orphaned file", "error", err)
		return
	}

	if len(orphanedFiles) == 0 {
		return
	}
	slog.Info(fmt.Sprintf("Ditemukan %d orphaned file.", len(orphanedFiles)))

	var idsToDeleteFromDB []int32
	for _, file := range orphanedFiles {
		var folder string
		switch file.Type {
		case constant.FileTypeAttachment:
			folder = cfg.DirAttachment
		case constant.FileTypeProfile:
			folder = cfg.DirProfile
		case constant.FileTypeThumbnail:
			folder = cfg.DirThumbnail
		default:
			folder = cfg.DirAttachment
		}

		filePath := filepath.Join(folder, file.Name)

		err := storage.Delete(filePath)

		if err == nil {
			idsToDeleteFromDB = append(idsToDeleteFromDB, file.ID)
		} else {
			slog.Error("Gagal hapus file storage", "path", filePath, "error", err)
		}
	}

	if len(idsToDeleteFromDB) > 0 {
		err := database.DB.Transaction(func(tx *gorm.DB) error {
			return tx.Where("id IN ?", idsToDeleteFromDB).Delete(&model.File{}).Error
		})
		if err != nil {
			slog.Error("Cleanup gagal pada tahap DB", "error", err)
		}
	}
}

func ProcessDeletionQueue(batchSize int, maxRetries int, storage *adapter.StorageAdapter) {
	slog.Info("Memulai pemroses antrean penghapusan file sumber...", "batch_size", batchSize)

	var queueItems []model.SourceFileToDelete
	err := database.DB.Where("failed_attempts < ?", maxRetries).
		Limit(batchSize).
		Find(&queueItems).Error
	if err != nil {
		slog.Error("Antrean Hapus: Gagal mengambil data.", "error", err)
		return
	}

	if len(queueItems) == 0 {
		return
	}

	var successCount, failedCount int
	for _, item := range queueItems {
		err := storage.Delete(item.SourcePath)

		if err == nil {
			database.DB.Delete(&item)
			successCount++
		} else {
			slog.Error("Antrean Hapus: Gagal menghapus file.", "path", item.SourcePath, "error", err)
			errorMessage := err.Error()
			database.DB.Model(&item).Updates(map[string]interface{}{
				"failed_attempts": gorm.Expr("failed_attempts + 1"),
				"last_error":      &errorMessage,
			})
			failedCount++
		}
	}

	slog.Info("Pemroses antrean penghapusan selesai.", "berhasil", successCount, "gagal", failedCount)
}

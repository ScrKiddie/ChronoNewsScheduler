package service

import (
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

func RunCleanupOldCompressedFiles(destDir string, threshold time.Duration, batchSize int) {
	slog.Info("Memulai scheduler cleanup untuk file .webp lama...", "batch_size", batchSize)

	thresholdTime := time.Now().Add(-threshold)
	unixThreshold := thresholdTime.Unix()

	var filesToDelete []model.File
	var deletedCount int
	var failedToDeleteDbCount int
	var failedToDeleteFileCount int

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("used_by_post_id IS NULL AND created_at < ?", unixThreshold).
			Limit(batchSize).
			Find(&filesToDelete).Error
		if err != nil {
			return fmt.Errorf("gagal mencari file .webp untuk dihapus: %w", err)
		}

		if len(filesToDelete) == 0 {
			slog.Info("Cleanup .webp: Tidak ada file lama yang tidak terpakai untuk dihapus.")
			return nil
		}

		slog.Info(fmt.Sprintf("Cleanup .webp: Ditemukan %d file untuk dihapus.", len(filesToDelete)))

		var idsToDelete []int32
		for _, file := range filesToDelete {
			originalName := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			webpFileName := fmt.Sprintf("%s.webp", originalName)
			filePath := filepath.Join(destDir, webpFileName)

			err := os.Remove(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Warn("Cleanup .webp: File sudah tidak ada di disk, akan tetap menghapus record DB", "file", filePath)
				} else {
					slog.Error("Cleanup .webp: Gagal menghapus file dari disk", "file", filePath, "error", err)
					failedToDeleteFileCount++
					continue
				}
			}
			idsToDelete = append(idsToDelete, file.ID)
		}

		if len(idsToDelete) == 0 {
			slog.Warn("Cleanup .webp: Tidak ada record database yang akan dihapus setelah proses penghapusan file.")
			return nil
		}

		result := tx.Where("id IN ?", idsToDelete).Delete(&model.File{})
		if result.Error != nil {
			failedToDeleteDbCount = len(filesToDelete) - int(result.RowsAffected)
			return fmt.Errorf("gagal menghapus record file .webp dari database: %w", result.Error)
		}

		deletedCount = int(result.RowsAffected)
		return nil
	})

	if err != nil {
		slog.Error("Scheduler cleanup .webp gagal dengan error", "error", err)
	}

	slog.Info("Scheduler cleanup .webp selesai.",
		"total_ditemukan", len(filesToDelete),
		"berhasil_dihapus", deletedCount,
		"gagal_hapus_file", failedToDeleteFileCount,
		"gagal_hapus_db", failedToDeleteDbCount,
	)
}

func ProcessSourceFileDeletionQueue(batchSize int, maxRetries int) {
	slog.Info("Memulai scheduler untuk antrean penghapusan file sumber...", "batch_size", batchSize, "max_retries", maxRetries)
	db := database.DB

	var entries []model.SourceFileToDelete
	err := db.Where("failed_attempts < ?", maxRetries).
		Limit(batchSize).
		Find(&entries).Error
	if err != nil {
		slog.Error("Gagal mengambil tugas dari antrean penghapusan", "error", err)
		return
	}

	if len(entries) == 0 {
		slog.Info("Antrean penghapusan file sumber kosong.")
		return
	}

	var successCount, failedCount int
	for _, entry := range entries {
		err := os.Remove(entry.SourcePath)
		if err == nil || os.IsNotExist(err) {
			if err != nil {
				slog.Warn("Antrean Hapus: File sumber sudah tidak ada di disk, menghapus dari antrean", "path", entry.SourcePath)
			}
			db.Delete(&entry)
			successCount++
		} else {
			slog.Error("Antrean Hapus: Gagal menghapus file sumber", "path", entry.SourcePath, "error", err)
			errorMessage := err.Error()
			db.Model(&entry).Updates(map[string]interface{}{
				"failed_attempts": gorm.Expr("failed_attempts + 1"),
				"last_error":      &errorMessage,
			})
			failedCount++
		}
	}

	slog.Info("Scheduler antrean penghapusan selesai.", "berhasil_diproses", successCount, "gagal_diproses", failedCount)
}

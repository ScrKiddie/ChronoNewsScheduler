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

func RunCleanupScheduler(destDir string, threshold time.Duration) {
	slog.Info("Memulai scheduler cleanup...")

	thresholdTime := time.Now().Add(-threshold)
	unixThreshold := thresholdTime.Unix()

	var filesToDelete []model.File
	var deletedCount int
	var failedToDeleteDbCount int
	var failedToDeleteFileCount int

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("used_by_post_id IS NULL AND created_at < ?", unixThreshold).Find(&filesToDelete).Error
		if err != nil {
			return fmt.Errorf("gagal mencari file untuk dihapus: %w", err)
		}

		if len(filesToDelete) == 0 {
			slog.Info("Cleanup: Tidak ada file lama yang tidak terpakai untuk dihapus.")
			return nil
		}

		slog.Info(fmt.Sprintf("Cleanup: Ditemukan %d file untuk dihapus.", len(filesToDelete)))

		var idsToDelete []int32
		for _, file := range filesToDelete {
			originalName := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			webpFileName := fmt.Sprintf("%s.webp", originalName)
			filePath := filepath.Join(destDir, webpFileName)

			err := os.Remove(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Warn("Cleanup: File sudah tidak ada di disk, akan tetap menghapus record DB", "file", filePath)
				} else {
					slog.Error("Cleanup: Gagal menghapus file dari disk", "file", filePath, "error", err)
					failedToDeleteFileCount++
					continue
				}
			}
			idsToDelete = append(idsToDelete, file.ID)
		}

		if len(idsToDelete) == 0 {
			slog.Warn("Cleanup: Tidak ada record database yang akan dihapus setelah proses penghapusan file.")
			return nil
		}
		
		result := tx.Where("id IN ?", idsToDelete).Delete(&model.File{})
		if result.Error != nil {
			failedToDeleteDbCount = len(filesToDelete) - int(result.RowsAffected)
			return fmt.Errorf("gagal menghapus record file dari database: %w", result.Error)
		}

		deletedCount = int(result.RowsAffected)
		return nil
	})

	if err != nil {
		slog.Error("Scheduler cleanup gagal dengan error", "error", err)
	}

	slog.Info("Scheduler cleanup selesai.",
		"total_ditemukan", len(filesToDelete),
		"berhasil_dihapus", deletedCount,
		"gagal_hapus_file", failedToDeleteFileCount,
		"gagal_hapus_db", failedToDeleteDbCount,
	)
}

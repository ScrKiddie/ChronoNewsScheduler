package service

import (
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

type DeletionStats struct {
	TotalDitemukan    int
	BerhasilHapusDisk int
	GagalHapusDisk    int
	BerhasilHapusDB   int
	FileSudahTidakAda int
}

func CleanupOrphanedFiles(destDir string, threshold time.Duration, batchSize int) {
	slog.Info("Memulai tugas pembersihan file yatim...", "threshold", threshold.String(), "batch_size", batchSize)

	stats := DeletionStats{}
	thresholdTime := time.Now().Add(-threshold)
	unixThreshold := thresholdTime.Unix()

	var orphanedFiles []model.File
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("used_by_post_id IS NULL AND created_at < ?", unixThreshold).
			Limit(batchSize).
			Find(&orphanedFiles).Error
		if err != nil {
			return fmt.Errorf("gagal mencari file yatim: %w", err)
		}

		stats.TotalDitemukan = len(orphanedFiles)
		if stats.TotalDitemukan == 0 {
			slog.Info("Pembersihan File Yatim: Tidak ada file untuk dihapus.")
			return nil
		}
		slog.Info(fmt.Sprintf("Pembersihan File Yatim: Ditemukan %d file untuk dihapus.", stats.TotalDitemukan))

		var idsToDeleteFromDB []int32
		for _, file := range orphanedFiles {
			filePath := filepath.Join(destDir, file.Name)

			err := os.Remove(filePath)
			if err == nil {
				slog.Debug("Pembersihan File Yatim: Berhasil hapus file disk.", "path", filePath)
				stats.BerhasilHapusDisk++
				idsToDeleteFromDB = append(idsToDeleteFromDB, file.ID)
			} else if os.IsNotExist(err) {
				slog.Warn("Pembersihan File Yatim: File sudah tidak ada di disk, akan tetap hapus record DB.", "path", filePath)
				stats.FileSudahTidakAda++
				idsToDeleteFromDB = append(idsToDeleteFromDB, file.ID)
			} else {
				slog.Error("Pembersihan File Yatim: Gagal menghapus file dari disk.", "path", filePath, "error", err)
				stats.GagalHapusDisk++
			}
		}

		if len(idsToDeleteFromDB) == 0 {
			slog.Warn("Pembersihan File Yatim: Tidak ada record database yang akan dihapus.")
			return nil
		}

		result := tx.Where("id IN ?", idsToDeleteFromDB).Delete(&model.File{})
		if result.Error != nil {
			return fmt.Errorf("gagal menghapus record file yatim dari database: %w", result.Error)
		}

		stats.BerhasilHapusDB = int(result.RowsAffected)
		return nil
	})

	if err != nil {
		slog.Error("Tugas pembersihan file yatim gagal.", "error", err)
	}

	slog.Info("Tugas pembersihan file yatim selesai.",
		"total_ditemukan", stats.TotalDitemukan,
		"berhasil_dihapus_total (db)", stats.BerhasilHapusDB,
		"gagal_hapus_disk", stats.GagalHapusDisk,
		"file_sudah_tidak_ada", stats.FileSudahTidakAda,
	)
}

func ProcessDeletionQueue(batchSize int, maxRetries int) {
	slog.Info("Memulai pemroses antrean penghapusan file sumber...", "batch_size", batchSize, "max_retries", maxRetries)

	var queueItems []model.SourceFileToDelete
	err := database.DB.Where("failed_attempts < ?", maxRetries).
		Limit(batchSize).
		Find(&queueItems).Error
	if err != nil {
		slog.Error("Antrean Hapus: Gagal mengambil data dari antrean.", "error", err)
		return
	}

	if len(queueItems) == 0 {
		slog.Info("Antrean Hapus: Antrean kosong, tidak ada tugas.")
		return
	}

	var successCount, failedCount int
	for _, item := range queueItems {
		err := os.Remove(item.SourcePath)
		if err == nil || os.IsNotExist(err) {
			if err != nil {
				slog.Warn("Antrean Hapus: File sumber sudah tidak ada, item dihapus dari antrean.", "path", item.SourcePath)
			}
			database.DB.Delete(&item)
			successCount++
		} else {
			slog.Error("Antrean Hapus: Gagal menghapus file sumber.", "path", item.SourcePath, "error", err)
			errorMessage := err.Error()
			database.DB.Model(&item).Updates(map[string]interface{}{
				"failed_attempts": gorm.Expr("failed_attempts + 1"),
				"last_error":      &errorMessage,
			})
			failedCount++
		}
	}

	slog.Info("Pemroses antrean penghapusan selesai.", "berhasil_diproses", successCount, "gagal_diproses", failedCount)
}

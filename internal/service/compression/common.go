package compression

import (
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"chrononews-scheduler/vips"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

func calculateOptimalScale(w, h int, maxWidth, maxHeight int) float64 {
	if w <= maxWidth && h <= maxHeight {
		return 1.0
	}
	return math.Min(float64(maxWidth)/float64(w), float64(maxHeight)/float64(h))
}

func handleSuccess(task model.File, cfg *config.Config) {
	if cfg.IsTestMode {
		slog.Debug("Mode Tes: Melewati pembaruan status 'compressed' di database.", "task_id", task.ID)
		return
	}

	originalNameWithoutExt := strings.TrimSuffix(task.Name, filepath.Ext(task.Name))
	newWebPFileName := fmt.Sprintf("%s.webp", originalNameWithoutExt)

	sourceFilePath := filepath.Join(cfg.SourceDir, task.Name)
	deletionEntry := model.SourceFileToDelete{
		FileID:     task.ID,
		SourcePath: sourceFilePath,
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&task).Updates(map[string]interface{}{
			"status":     "compressed",
			"last_error": nil,
			"name":       newWebPFileName,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&deletionEntry).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		slog.Error("KRITIS: Gagal menyelesaikan transaksi sukses kompresi", "task_id", task.ID, "error", err)
	}
}

func handleFailure(task model.File, err error, cfg *config.Config) {
	if cfg.IsTestMode {
		slog.Debug("Mode Tes: Melewati pembaruan status kegagalan di database.", "task_id", task.ID)
		return
	}

	newAttempts := task.FailedAttempts + 1
	errorMessage := err.Error()

	if newAttempts >= cfg.MaxRetries {
		slog.Error("Tugas gagal permanen, dipindahkan ke DLQ", "file_name", task.Name, "error", errorMessage)

		tx := database.DB.Begin()
		err := tx.Model(&task).Updates(map[string]interface{}{
			"status":          "failed",
			"failed_attempts": newAttempts,
			"last_error":      &errorMessage,
		}).Error
		if err != nil {
			tx.Rollback()
			slog.Error("KRITIS: Gagal update status FAILED", "task_id", task.ID, "error", err)
			return
		}

		dlqEntry := model.DeadLetterQueue{FileID: task.ID, ErrorMessage: errorMessage}
		err = tx.Create(&dlqEntry).Error
		if err != nil {
			tx.Rollback()
			slog.Error("KRITIS: Gagal memasukkan tugas ke DLQ", "task_id", task.ID, "error", err)
			return
		}

		tx.Commit()
	} else {
		slog.Warn("Tugas gagal, akan dicoba lagi di jadwal berikutnya", "file_name", task.Name, "attempts", newAttempts)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"status":          "pending",
			"failed_attempts": newAttempts,
			"last_error":      &errorMessage,
		})
	}
}

func processImageWithReader(reader io.ReadCloser, cfg *config.Config) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer reader.Close()

		source := vips.NewSource(reader)
		defer source.Close()

		img, err := vips.NewImageFromSource(source, &vips.LoadOptions{
			Access:      vips.AccessSequentialUnbuffered,
			FailOnError: true,
		})
		if err != nil {
			pw.CloseWithError(fmt.Errorf("gagal membuat image dari source: %w", err))
			return
		}
		defer img.Close()

		w, h := img.Width(), img.Height()
		scale := calculateOptimalScale(w, h, cfg.MaxWidth, cfg.MaxHeight)

		if scale < 1.0 {
			if err = img.Resize(scale, nil); err != nil {
				pw.CloseWithError(fmt.Errorf("gagal resize: %w", err))
				return
			}
		}

		target := vips.NewTarget(pw)
		defer target.Close()

		err = img.WebpsaveTarget(target, &vips.WebpsaveTargetOptions{
			Q: cfg.WebPQuality,
		})
		if err != nil {
			return
		}
	}()
	return pr, nil
}

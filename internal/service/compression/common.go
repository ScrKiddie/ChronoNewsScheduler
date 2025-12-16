package compression

import (
	"bytes"
	"chrononews-scheduler/internal/adapter"
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/constant"
	"chrononews-scheduler/internal/database"
	"chrononews-scheduler/internal/model"
	"chrononews-scheduler/vips"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

func resolvePath(cfg *config.Config, fileType, fileName string) string {
	var folder string
	switch fileType {
	case constant.FileTypeAttachment:
		folder = cfg.DirAttachment
	case constant.FileTypeProfile:
		folder = cfg.DirProfile
	case constant.FileTypeThumbnail:
		folder = cfg.DirThumbnail
	default:
		folder = cfg.DirAttachment
	}
	return filepath.Join(folder, fileName)
}

func ExecuteCompressionTask(ctx context.Context, cfg *config.Config, task model.File, storage *adapter.StorageAdapter) error {
	sourcePath := resolvePath(cfg, task.Type, task.Name)
	originalName := strings.TrimSuffix(task.Name, filepath.Ext(task.Name))
	newFileName := fmt.Sprintf("%s.webp", originalName)
	outputPath := resolvePath(cfg, task.Type, newFileName)

	reader, err := storage.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("gagal membuka source (%s): %w", sourcePath, err)
	}

	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("Gagal menutup reader source", "path", sourcePath, "error", err)
		}
	}()

	processedReader, err := processImageWithReader(reader, cfg)
	if err != nil {
		return fmt.Errorf("gagal menyiapkan proses gambar: %w", err)
	}

	defer func() {
		if err := processedReader.Close(); err != nil {
			slog.Warn("Gagal menutup processed reader", "error", err)
		}
	}()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, processedReader)
	if err != nil {
		return fmt.Errorf("gagal mem-buffer hasil kompresi: %w", err)
	}

	if cfg.IsTestMode {
		slog.Debug("TEST MODE: Simulasi sukses. File tidak disimpan.", "mock_path", outputPath, "size_bytes", buf.Len())
		return nil
	}

	errChan := make(chan error, 1)
	go func() {
		uErr := storage.Put(outputPath, &buf, "image/webp")
		errChan <- uErr
	}()

	select {
	case <-ctx.Done():
		go func() {
			if err := storage.Delete(outputPath); err != nil {
				slog.Warn("Gagal cleanup file (timeout)", "path", outputPath, "error", err)
			}
		}()
		return ctx.Err()
	case err := <-errChan:
		if err != nil {
			go func() {
				if err := storage.Delete(outputPath); err != nil {
					slog.Warn("Gagal cleanup file (upload fail)", "path", outputPath, "error", err)
				}
			}()
			return fmt.Errorf("gagal menyimpan hasil: %w", err)
		}
	}

	return nil
}

func handleSuccess(task model.File, cfg *config.Config) {
	if cfg.IsTestMode {
		slog.Debug("TEST MODE: Skip update DB.", "task_id", task.ID)
		return
	}

	originalNameWithoutExt := strings.TrimSuffix(task.Name, filepath.Ext(task.Name))
	newWebPFileName := fmt.Sprintf("%s.webp", originalNameWithoutExt)
	sourcePath := resolvePath(cfg, task.Type, task.Name)

	deletionEntry := model.SourceFileToDelete{
		FileID:     task.ID,
		SourcePath: sourcePath,
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
		slog.Error("KRITIS: Gagal transaksi sukses", "error", err)
	}
}

func handleFailure(task model.File, err error, cfg *config.Config) {
	if cfg.IsTestMode {
		slog.Error("TEST MODE: Simulasi Gagal.", "error", err)
		return
	}

	newAttempts := task.FailedAttempts + 1
	errorMessage := err.Error()

	if len(errorMessage) > 250 {
		errorMessage = errorMessage[:250] + "..."
	}

	if newAttempts >= cfg.MaxRetries {
		slog.Error("Tugas gagal permanen -> DLQ", "file", task.Name)
		tx := database.DB.Begin()
		if err := tx.Model(&task).Updates(map[string]interface{}{
			"status": "failed", "failed_attempts": newAttempts, "last_error": &errorMessage,
		}).Error; err != nil {
			tx.Rollback()
			return
		}
		tx.Create(&model.DeadLetterQueue{FileID: task.ID, ErrorMessage: errorMessage})
		tx.Commit()
	} else {
		slog.Warn("Tugas gagal, retry next schedule", "attempts", newAttempts)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"status": "pending", "failed_attempts": newAttempts, "last_error": &errorMessage,
		})
	}
}

func processImageWithReader(reader io.ReadCloser, cfg *config.Config) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {

		defer func() {
			if err := pw.Close(); err != nil {
				slog.Warn("Gagal menutup pipe writer", "error", err)
			}
		}()

		defer func() {
			if err := reader.Close(); err != nil {
				slog.Warn("Gagal menutup reader input di goroutine", "error", err)
			}
		}()

		source := vips.NewSource(reader)

		defer source.Close()

		img, err := vips.NewImageFromSource(source, &vips.LoadOptions{
			Access:      vips.AccessSequentialUnbuffered,
			FailOnError: true,
		})
		if err != nil {
			pw.CloseWithError(fmt.Errorf("vips load: %w", err))
			return
		}

		defer img.Close()

		w, h := img.Width(), img.Height()
		scale := calculateOptimalScale(w, h, cfg.MaxWidth, cfg.MaxHeight)

		if scale < 1.0 {
			if err = img.Resize(scale, nil); err != nil {
				pw.CloseWithError(fmt.Errorf("vips resize: %w", err))
				return
			}
		}

		target := vips.NewTarget(pw)

		defer target.Close()

		err = img.WebpsaveTarget(target, &vips.WebpsaveTargetOptions{Q: cfg.WebPQuality})
		if err != nil {
			slog.Warn("Gagal menyimpan target webp", "error", err)
			return
		}
	}()
	return pr, nil
}

func calculateOptimalScale(w, h int, maxWidth, maxHeight int) float64 {
	if w <= maxWidth && h <= maxHeight {
		return 1.0
	}
	return math.Min(float64(maxWidth)/float64(w), float64(maxHeight)/float64(h))
}

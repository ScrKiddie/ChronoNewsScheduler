package service

import (
	"chrononews-scheduler/internal/model"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func runSequential(ctx context.Context, tasks []model.File, cfg SchedulerConfig) {
	var successfulCount int
	var failedCount int

	for _, task := range tasks {
		select {
		case <-ctx.Done():
			slog.Info("Proses sekuensial dibatalkan oleh sinyal shutdown.")
			slog.Info("Hasil parsial proses sekuensial.",
				"berhasil", successfulCount,
				"gagal", failedCount,
			)
			return
		default:
		}

		slog.Debug("Memproses file", "mode", "sekuensial", "file_name", task.Name)
		err := compressImageStreaming(cfg, task)
		if err != nil {
			failedCount++
			handleFailure(task, err, cfg)
		} else {
			successfulCount++
			handleSuccess(task, cfg)
		}
	}

	slog.Info("Proses sekuensial selesai.",
		"berhasil", successfulCount,
		"gagal", failedCount,
	)
}

func compressImageStreaming(cfg SchedulerConfig, task model.File) error {
	sourceFile := filepath.Join(cfg.SourceDir, task.Name)
	file, err := os.Open(sourceFile)
	if err != nil {
		return fmt.Errorf("gagal membuka file sumber: %w", err)
	}
	defer file.Close()

	processedReader, err := processImageWithReader(file, cfg)
	if err != nil {
		return fmt.Errorf("gagal memproses gambar: %w", err)
	}
	defer processedReader.Close()

	originalName := task.Name[:len(task.Name)-len(filepath.Ext(task.Name))]
	newFileName := fmt.Sprintf("%s.webp", originalName)
	outputFilePath := filepath.Join(cfg.DestDir, newFileName)

	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("gagal membuat file tujuan: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, processedReader); err != nil {
		return fmt.Errorf("gagal menulis hasil ke file: %w", err)
	}
	return nil
}

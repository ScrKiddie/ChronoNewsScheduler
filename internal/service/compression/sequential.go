package compression

import (
	"chrononews-scheduler/internal/adapter"
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/model"
	"context"
	"log/slog"
)

func runSequential(ctx context.Context, tasks []model.File, cfg *config.Config, storage *adapter.StorageAdapter) {
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

		err := ExecuteCompressionTask(ctx, cfg, task, storage)

		if err != nil {
			failedCount++
			slog.Error("Gagal memproses file", "file", task.Name, "error", err)
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

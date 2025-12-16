package compression

import (
	"chrononews-scheduler/internal/adapter"
	"chrononews-scheduler/internal/config"
	"chrononews-scheduler/internal/model"
	"context"
	"log/slog"
	"sync"
)

type simpleJob struct {
	task model.File
}

type processResult struct {
	task model.File
	err  error
}

func runWorkerPool(ctx context.Context, tasks []model.File, cfg *config.Config, storage *adapter.StorageAdapter) {
	numWorkers := cfg.NumWorkers
	if numWorkers <= 0 {
		numWorkers = 1
	}

	jobs := make(chan simpleJob, len(tasks))
	results := make(chan processResult, len(tasks))

	var wg sync.WaitGroup

	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go simpleWorker(ctx, jobs, results, &wg, cfg, i, storage)
	}

	go func() {
	DispatchLoop:
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				slog.Warn("Shutdown diminta, berhenti mengirim tugas ke worker pool.")
				break DispatchLoop
			case jobs <- simpleJob{task: task}:
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var successfulCount int
	var failedCount int

	for result := range results {
		if result.err != nil {
			failedCount++
			handleFailure(result.task, result.err, cfg)
		} else {
			successfulCount++
			handleSuccess(result.task, cfg)
		}
	}

	slog.Info("Proses worker pool selesai.",
		"berhasil", successfulCount,
		"gagal", failedCount,
		"workers", numWorkers,
	)
}

func simpleWorker(
	ctx context.Context,
	jobs <-chan simpleJob,
	results chan<- processResult,
	wg *sync.WaitGroup,
	cfg *config.Config,
	workerID int,
	storage *adapter.StorageAdapter,
) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			slog.Debug("Worker memproses file",
				"worker_id", workerID,
				"file", job.task.Name,
			)

			err := ExecuteCompressionTask(ctx, cfg, job.task, storage)

			select {
			case <-ctx.Done():
				return
			case results <- processResult{task: job.task, err: err}:
			}
		}
	}
}

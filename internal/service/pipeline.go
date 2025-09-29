package service

import (
	"chrononews-scheduler/internal/model"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type readJob struct {
	task      model.File
	sourceDir string
}

type processJob struct {
	task   model.File
	reader io.ReadCloser
	err    error
}

type writeJob struct {
	task        model.File
	reader      io.ReadCloser
	destination string
	err         error
}

type processResult struct {
	task model.File
	err  error
}

func runConcurrentPipeline(ctx context.Context, tasks []model.File, cfg SchedulerConfig) {
	readJobs := make(chan readJob, len(tasks))
	processQueue := make(chan processJob, cfg.NumIOWorkers)
	writeQueue := make(chan writeJob, cfg.NumCPUWorkers)
	results := make(chan processResult, len(tasks))

	var readerWg, processorWg, writerWg sync.WaitGroup

	readerWg.Add(cfg.NumIOWorkers)
	for i := 1; i <= cfg.NumIOWorkers; i++ {
		go readerWorker(ctx, readJobs, processQueue, &readerWg)
	}

	processorWg.Add(cfg.NumCPUWorkers)
	for i := 1; i <= cfg.NumCPUWorkers; i++ {
		go processorWorker(ctx, processQueue, writeQueue, &processorWg, cfg)
	}

	writerWg.Add(cfg.NumIOWorkers)
	for i := 1; i <= cfg.NumIOWorkers; i++ {
		go writerWorker(ctx, writeQueue, results, &writerWg)
	}

SendLoop:
	for _, task := range tasks {
		select {
		case <-ctx.Done():
			slog.Warn("Shutdown diminta, berhenti mengirim tugas baru ke pipeline.")
			break SendLoop
		case readJobs <- readJob{task: task, sourceDir: cfg.SourceDir}:
		}
	}

	close(readJobs)

	go func() {
		readerWg.Wait()
		close(processQueue)
		processorWg.Wait()
		close(writeQueue)
		writerWg.Wait()
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
	slog.Info("Proses pipeline selesai.", "berhasil", successfulCount, "gagal", failedCount)
}

func readerWorker(ctx context.Context, jobs <-chan readJob, processQueue chan<- processJob, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			sourceFile := filepath.Join(job.sourceDir, job.task.Name)
			file, err := os.Open(sourceFile)
			if err != nil {
				err = fmt.Errorf("reader: %w", err)
				slog.Warn(err.Error(), "file_name", job.task.Name)
				processQueue <- processJob{task: job.task, err: err}
				continue
			}
			processQueue <- processJob{task: job.task, reader: file}
		}
	}
}

func processorWorker(ctx context.Context, processQueue <-chan processJob, writeQueue chan<- writeJob, wg *sync.WaitGroup, cfg SchedulerConfig) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-processQueue:
			if !ok {
				return
			}
			if job.err != nil {
				writeQueue <- writeJob{task: job.task, err: job.err}
				continue
			}

			processedReader, err := processImageWithReader(job.reader, cfg)
			if err != nil {
				err = fmt.Errorf("processor: %w", err)
				slog.Warn(err.Error(), "file_name", job.task.Name)
				writeQueue <- writeJob{task: job.task, err: err}
				continue
			}

			originalName := job.task.Name[:len(job.task.Name)-len(filepath.Ext(job.task.Name))]
			newFileName := fmt.Sprintf("%s.webp", originalName)
			outputFilePath := filepath.Join(cfg.DestDir, newFileName)

			writeQueue <- writeJob{
				task:        job.task,
				reader:      processedReader,
				destination: outputFilePath,
			}
		}
	}
}

func writerWorker(ctx context.Context, writeQueue <-chan writeJob, results chan<- processResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-writeQueue:
			if !ok {
				return
			}
			if job.err != nil {
				results <- processResult{task: job.task, err: job.err}
				continue
			}

			outFile, err := os.Create(job.destination)
			if err != nil {
				results <- processResult{task: job.task, err: fmt.Errorf("writer: gagal membuat file tujuan: %w", err)}
				continue
			}

			_, err = io.Copy(outFile, job.reader)
			if closeErr := outFile.Close(); err == nil && closeErr != nil {
				err = closeErr
			}

			if err != nil {
				results <- processResult{task: job.task, err: fmt.Errorf("writer: gagal menulis/menutup file: %w", err)}
				continue
			}

			results <- processResult{task: job.task, err: nil}
		}
	}
}

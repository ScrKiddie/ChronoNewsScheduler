package adapter

import (
	"chrononews-scheduler/internal/config"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type StorageAdapter struct {
	mode   string
	client *s3.Client
	bucket string
}

func NewStorageAdapter(cfg *config.Config, s3Client *s3.Client) *StorageAdapter {
	return &StorageAdapter{
		mode:   cfg.StorageMode,
		client: s3Client,
		bucket: cfg.S3Bucket,
	}
}

func (s *StorageAdapter) Open(path string) (io.ReadCloser, error) {
	if s.mode == "s3" {
		if s.client == nil {
			return nil, fmt.Errorf("s3 client is not initialized")
		}
		key := filepath.ToSlash(path)

		output, err := s.client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return nil, err
		}
		return output.Body, nil
	}

	return os.Open(path)
}

func (s *StorageAdapter) Put(path string, reader io.Reader, contentType string) error {
	if s.mode == "s3" {
		if s.client == nil {
			return fmt.Errorf("s3 client is not initialized")
		}
		key := filepath.ToSlash(path)

		_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(key),
			Body:        reader,
			ContentType: aws.String(contentType),
		})
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	outFile, err := os.Create(path)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(outFile, reader)

	closeErr := outFile.Close()

	if copyErr != nil {
		return copyErr
	}

	return closeErr
}

func (s *StorageAdapter) Delete(path string) error {
	if s.mode == "s3" {
		if s.client == nil {
			return fmt.Errorf("s3 client is not initialized")
		}
		key := filepath.ToSlash(path)
		_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		})
		return err
	}

	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("File lokal tidak ditemukan saat penghapusan", "path", path)
			return nil
		}
		return err
	}
	return nil
}

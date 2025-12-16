package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const webpMaxDimension = 16383

type Config struct {
	DSN string

	DBSSLMode string

	LogLevel     string
	AppMode      string
	AppSchedule  string
	IsConcurrent bool
	IsTestMode   bool
	BatchSize    int

	DirAttachment string
	DirProfile    string
	DirThumbnail  string

	NumWorkers int

	StorageMode string
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string

	WebPQuality             int
	MaxWidth                int
	MaxHeight               int
	MaxRetries              int
	CleanupThreshold        time.Duration
	CleanupBatchSize        int
	JanitorStuckThreshold   time.Duration
	DeletionQueueBatchSize  int
	DeletionQueueMaxRetries int
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
func getEnvAsInt(key string, fallback int) (int, error) {
	strValue := getEnv(key, "")
	if strValue == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strValue)
	if err != nil {
		return 0, fmt.Errorf("env var %s: invalid integer value '%s'", key, strValue)
	}
	return value, nil
}
func getEnvAsBool(key string, fallback bool) (bool, error) {
	strValue := getEnv(key, "")
	if strValue == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strValue)
	if err != nil {
		return false, fmt.Errorf("env var %s: invalid boolean value '%s'", key, strValue)
	}
	return value, nil
}
func getEnvAsDuration(key string, fallback time.Duration) (time.Duration, error) {
	strValue := getEnv(key, "")
	if strValue == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(strValue)
	if err != nil {
		return 0, fmt.Errorf("env var %s: invalid duration value '%s'", key, strValue)
	}
	return value, nil
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	var err error

	cfg.AppSchedule = getEnv("APP_SCHEDULE", "")
	if cfg.AppSchedule == "" {
		return nil, fmt.Errorf("APP_SCHEDULE wajib diisi")
	}

	cfg.DirAttachment = getEnv("DIR_ATTACHMENT", "post_picture")
	cfg.DirProfile = getEnv("DIR_PROFILE", "profile_picture")
	cfg.DirThumbnail = getEnv("DIR_THUMBNAIL", "thumbnail")

	cfg.StorageMode = strings.ToLower(getEnv("STORAGE_MODE", "local"))
	cfg.S3Bucket = getEnv("S3_BUCKET", "")
	cfg.S3Region = getEnv("S3_REGION", "ap-southeast-1")
	cfg.S3AccessKey = getEnv("S3_ACCESS_KEY", "")
	cfg.S3SecretKey = getEnv("S3_SECRET_KEY", "")
	cfg.S3Endpoint = getEnv("S3_ENDPOINT", "")

	if cfg.StorageMode == "s3" {
		if cfg.S3Bucket == "" || cfg.S3Region == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
			return nil, fmt.Errorf("mode s3 aktif, wajib isi config S3 lengkap")
		}
	}

	cfg.DBSSLMode = getEnv("DB_SSL_MODE", "disable")

	cfg.DSN = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta lock_timeout=5000",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_USER", "user"),
		getEnv("DB_PASSWORD", "password"),
		getEnv("DB_NAME", "dbname"),
		getEnv("DB_PORT", "5432"),
		cfg.DBSSLMode,
	)

	cfg.LogLevel = getEnv("LOG_LEVEL", "info")
	cfg.AppMode = getEnv("APP_MODE", "all")

	if cfg.IsTestMode, err = getEnvAsBool("COMPRESSION_IS_TEST_MODE", false); err != nil {
		return nil, err
	}
	if cfg.MaxRetries, err = getEnvAsInt("COMPRESSION_MAX_RETRIES", 3); err != nil {
		return nil, err
	}
	if cfg.IsConcurrent, err = getEnvAsBool("COMPRESSION_IS_CONCURRENT", true); err != nil {
		return nil, err
	}
	if cfg.BatchSize, err = getEnvAsInt("COMPRESSION_BATCH_SIZE", 50); err != nil {
		return nil, err
	}
	if cfg.NumWorkers, err = getEnvAsInt("COMPRESSION_NUM_WORKERS", runtime.NumCPU()); err != nil {
		return nil, err
	}
	if cfg.WebPQuality, err = getEnvAsInt("COMPRESSION_WEBP_QUALITY", 75); err != nil {
		return nil, err
	}
	if cfg.MaxWidth, err = getEnvAsInt("COMPRESSION_MAX_WIDTH", 1980); err != nil {
		return nil, err
	}
	if cfg.MaxHeight, err = getEnvAsInt("COMPRESSION_MAX_HEIGHT", 1980); err != nil {
		return nil, err
	}

	if cfg.CleanupThreshold, err = getEnvAsDuration("CLEANUP_THRESHOLD", 30*24*time.Hour); err != nil {
		return nil, err
	}
	if cfg.CleanupBatchSize, err = getEnvAsInt("CLEANUP_BATCH_SIZE", 100); err != nil {
		return nil, err
	}
	if cfg.JanitorStuckThreshold, err = getEnvAsDuration("JANITOR_STUCK_THRESHOLD", 15*time.Minute); err != nil {
		return nil, err
	}
	if cfg.DeletionQueueBatchSize, err = getEnvAsInt("DELETION_QUEUE_BATCH_SIZE", 100); err != nil {
		return nil, err
	}
	if cfg.DeletionQueueMaxRetries, err = getEnvAsInt("DELETION_QUEUE_MAX_RETRIES", 5); err != nil {
		return nil, err
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateConfig(cfg *Config) error {
	validAppModes := map[string]bool{"all": true, "compression": true, "cleanup": true, "janitor": true, "deletion": true}
	if !validAppModes[strings.ToLower(cfg.AppMode)] {
		return fmt.Errorf("APP_MODE tidak valid: '%s'", cfg.AppMode)
	}
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("BATCH_SIZE error")
	}
	if cfg.NumWorkers <= 0 {
		return fmt.Errorf("NUM_WORKERS error")
	}
	if cfg.WebPQuality < 1 || cfg.WebPQuality > 100 {
		return fmt.Errorf("WEBP_QUALITY harus di antara 1 dan 100")
	}
	if cfg.MaxWidth <= 0 || cfg.MaxHeight <= 0 {
		return fmt.Errorf("MAX_WIDTH dan MAX_HEIGHT harus lebih besar dari 0")
	}
	if cfg.MaxWidth > webpMaxDimension || cfg.MaxHeight > webpMaxDimension {
		return fmt.Errorf("MAX_WIDTH atau MAX_HEIGHT melebihi batas WebP (%dpx)", webpMaxDimension)
	}

	if cfg.StorageMode == "local" {
		dirsToCheck := []string{cfg.DirAttachment, cfg.DirProfile, cfg.DirThumbnail}
		for _, dir := range dirsToCheck {
			if dir == "" || dir == "." {
				continue
			}
			info, err := os.Stat(dir)
			if os.IsNotExist(err) {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("gagal membuat folder '%s'", dir)
				}
			} else if err != nil {
				return err
			} else if !info.IsDir() {
				return fmt.Errorf("path '%s' bukan folder", dir)
			}
		}
	}
	return nil
}

package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const webpMaxDimension = 16383

type Config struct {
	DSN           string
	LogLevel      string
	IsConcurrent  bool
	IsTestMode    bool
	BatchSize     int
	SourceDir     string
	DestDir       string
	Schedule      string
	NumIOWorkers  int
	NumCPUWorkers int
	WebPQuality   int
	MaxWidth      int
	MaxHeight     int
	MaxRetries    int
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

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	var err error

	cfg.DSN = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Jakarta",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_USER", "user"),
		getEnv("DB_PASSWORD", "password"),
		getEnv("DB_NAME", "dbname"),
		getEnv("DB_PORT", "5432"),
	)

	cfg.LogLevel = getEnv("LOG_LEVEL", "info")
	cfg.Schedule = getEnv("SCHEDULE", "")
	cfg.SourceDir = getEnv("SOURCE_DIR", "./images/source")
	cfg.DestDir = getEnv("DEST_DIR", "./images/compressed")

	if cfg.IsTestMode, err = getEnvAsBool("IS_TEST_MODE", false); err != nil {
		return nil, err
	}

	if cfg.IsConcurrent, err = getEnvAsBool("IS_CONCURRENT", true); err != nil {
		return nil, err
	}
	if cfg.BatchSize, err = getEnvAsInt("BATCH_SIZE", 50); err != nil {
		return nil, err
	}
	if cfg.NumIOWorkers, err = getEnvAsInt("NUM_IO_WORKERS", runtime.NumCPU()*2); err != nil {
		return nil, err
	}
	if cfg.NumCPUWorkers, err = getEnvAsInt("NUM_CPU_WORKERS", runtime.NumCPU()); err != nil {
		return nil, err
	}
	if cfg.MaxRetries, err = getEnvAsInt("MAX_RETRIES", 3); err != nil {
		return nil, err
	}
	if cfg.WebPQuality, err = getEnvAsInt("WEBP_QUALITY", 75); err != nil {
		return nil, err
	}
	if cfg.MaxWidth, err = getEnvAsInt("MAX_WIDTH", 1980); err != nil {
		return nil, err
	}
	if cfg.MaxHeight, err = getEnvAsInt("MAX_HEIGHT", 1980); err != nil {
		return nil, err
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("BATCH_SIZE harus lebih besar dari 0")
	}
	if cfg.NumIOWorkers <= 0 {
		return fmt.Errorf("NUM_IO_WORKERS harus lebih besar dari 0")
	}
	if cfg.NumCPUWorkers <= 0 {
		return fmt.Errorf("NUM_CPU_WORKERS harus lebih besar dari 0")
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
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("MAX_RETRIES tidak boleh negatif")
	}

	for _, dir := range []string{cfg.SourceDir, cfg.DestDir} {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return fmt.Errorf("direktori '%s' tidak ditemukan", dir)
		}
		if err != nil {
			return fmt.Errorf("gagal memeriksa direktori '%s': %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("path '%s' bukanlah sebuah direktori", dir)
		}
	}

	validLogLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
	if !validLogLevels[strings.ToUpper(cfg.LogLevel)] {
		return fmt.Errorf("LOG_LEVEL tidak valid: '%s'. Gunakan salah satu dari: debug, info, warn, error", cfg.LogLevel)
	}

	return nil
}

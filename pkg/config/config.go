package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ElevenLabsAPIKey      string
	ElevenLabsBaseURL     string
	ElevenLabsEnhancePath string
	ReplicateAPIToken     string
	ReplicateBaseURL      string
	ReplicateModel        string
	ReplicatePreferWait   bool
	TranscribeEnabled     bool
	WhisperDockerPath     string
	WhisperDockerImage    string
	WhisperModel          string
	WhisperModelDir       string
	WhisperAutoDownload   bool
	OutputDir             string
	FFmpegPath            string
	RecordFormat          string
	RecordDevice          string
	RecordDurationSeconds int
	JobPollInterval       time.Duration
	HTTPTimeout           time.Duration
}

func Load() Config {
	return Config{
		ElevenLabsAPIKey:      getEnv("ELEVENLABS_API_KEY", ""),
		ElevenLabsBaseURL:     getEnv("ELEVENLABS_BASE_URL", "https://api.elevenlabs.io"),
		ElevenLabsEnhancePath: getEnv("ELEVENLABS_ENHANCE_PATH", "/v1/audio-isolation"),
		OutputDir:             getEnv("OUTPUT_DIR", "./outputs"),
		FFmpegPath:            getEnv("FFMPEG_PATH", "ffmpeg"),
		ReplicateAPIToken:     getEnv("REPLICATE_API_TOKEN", ""),
		ReplicateBaseURL:      getEnv("REPLICATE_BASE_URL", "https://api.replicate.com/v1"),
		ReplicateModel:        getEnv("REPLICATE_MODEL", "minimax/video-01"),
		ReplicatePreferWait:   getEnvBool("REPLICATE_PREFER_WAIT", true),
		TranscribeEnabled:     getEnvBool("TRANSCRIBE_ENABLED", true),
		WhisperDockerPath:     getEnv("WHISPER_DOCKER_PATH", "docker"),
		WhisperDockerImage:    getEnv("WHISPER_DOCKER_IMAGE", "ghcr.io/ggml-org/whisper.cpp:main"),
		WhisperModel:          getEnv("WHISPER_MODEL", "small"),
		WhisperModelDir:       getEnv("WHISPER_MODEL_DIR", "./models"),
		WhisperAutoDownload:   getEnvBool("WHISPER_AUTO_DOWNLOAD", true),
		RecordFormat:          getEnv("AUDIO_RECORD_FORMAT", "alsa"),
		RecordDevice:          getEnv("AUDIO_RECORD_DEVICE", "default"),
		RecordDurationSeconds: getEnvInt("AUDIO_RECORD_SECONDS", 15),
		JobPollInterval:       getEnvDuration("JOB_POLL_INTERVAL", 4*time.Second),
		HTTPTimeout:           getEnvDuration("HTTP_TIMEOUT", 5*time.Minute),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

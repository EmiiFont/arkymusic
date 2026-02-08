package main

import (
	"log"

	"github.com/audio2videoAI/internal/ai/elevenlabs"
	"github.com/audio2videoAI/internal/ai/replicate"
	"github.com/audio2videoAI/internal/audio"
	"github.com/audio2videoAI/internal/jobs"
	"github.com/audio2videoAI/internal/tui"
	"github.com/audio2videoAI/pkg/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ell := elevenlabs.NewClient(cfg.ElevenLabsAPIKey, cfg.ElevenLabsBaseURL, cfg.ElevenLabsEnhancePath, cfg.HTTPTimeout)
	replicateClient := replicate.NewClient(cfg.ReplicateAPIToken, cfg.ReplicateBaseURL, cfg.ReplicateModel, cfg.HTTPTimeout)
	jobRunner := &jobs.Runner{
		ElevenLabs: ell,
		Replicate:  replicateClient,
		Transcribe: audio.TranscribeConfig{
			Enabled:      cfg.TranscribeEnabled,
			DockerPath:   cfg.WhisperDockerPath,
			DockerImage:  cfg.WhisperDockerImage,
			Model:        cfg.WhisperModel,
			ModelDir:     cfg.WhisperModelDir,
			AutoDownload: cfg.WhisperAutoDownload,
		},
		FFmpegPath:   cfg.FFmpegPath,
		PollInterval: cfg.JobPollInterval,
		PreferWait:   cfg.ReplicatePreferWait,
	}

	program := tea.NewProgram(tui.NewModel(cfg, jobRunner), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}

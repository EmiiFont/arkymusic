package audio

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type TranscribeConfig struct {
	Enabled      bool
	DockerPath   string
	DockerImage  string
	Model        string
	ModelDir     string
	AutoDownload bool
}

func Transcribe(ctx context.Context, config TranscribeConfig, audioPath, outputDir string) (string, string, error) {
	if !config.Enabled {
		return "", "", nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", err
	}

	if config.DockerPath == "" {
		config.DockerPath = "docker"
	}
	if config.DockerImage == "" {
		config.DockerImage = "ghcr.io/ggerganov/whisper.cpp:1.5.5"
	}
	if config.Model == "" {
		config.Model = "small"
	}
	if config.ModelDir == "" {
		config.ModelDir = "./models"
	}

	modelDir, err := filepath.Abs(config.ModelDir)
	if err != nil {
		return "", "", err
	}
	config.ModelDir = modelDir

	modelPath := filepath.Join(config.ModelDir, fmt.Sprintf("ggml-%s.bin", config.Model))
	if _, err := os.Stat(modelPath); err != nil {
		if !config.AutoDownload {
			return "", "", fmt.Errorf("whisper model not found: %s", modelPath)
		}
		if err := downloadModel(ctx, config); err != nil {
			return "", "", err
		}
	}

	workDir := filepath.Join(outputDir, fmt.Sprintf("whisper-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", "", err
	}
	defer os.RemoveAll(workDir)

	workDirAbs, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", err
	}

	inputPath := filepath.Join(workDirAbs, "input.wav")
	if err := copyFile(audioPath, inputPath); err != nil {
		return "", "", err
	}

	outputBase := filepath.Join(workDir, "transcript")
	cmd := exec.CommandContext(
		ctx,
		config.DockerPath,
		"run",
		"--rm",
		"-v", fmt.Sprintf("%s:/work", workDirAbs),
		"-v", fmt.Sprintf("%s:/models", config.ModelDir),
		config.DockerImage,
		"./main",
		"-m", fmt.Sprintf("/models/ggml-%s.bin", config.Model),
		"-f", "/work/input.wav",
		"-of", "/work/transcript",
		"-otxt",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("whisper transcription failed: %s", strings.TrimSpace(string(output)))
	}

	transcriptPath := outputBase + ".txt"
	content, err := os.ReadFile(transcriptPath)
	if err != nil {
		return "", "", err
	}

	finalPath := filepath.Join(outputDir, fmt.Sprintf("transcript-%d.txt", time.Now().UnixNano()))
	if err := os.Rename(transcriptPath, finalPath); err != nil {
		return "", "", err
	}

	return strings.TrimSpace(string(content)), finalPath, nil
}

func downloadModel(ctx context.Context, config TranscribeConfig) error {
	cmd := exec.CommandContext(
		ctx,
		config.DockerPath,
		"run",
		"--rm",
		"-v", fmt.Sprintf("%s:/models", config.ModelDir),
		config.DockerImage,
		"./models/download-ggml-model.sh",
		config.Model,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("whisper model download failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return nil
}

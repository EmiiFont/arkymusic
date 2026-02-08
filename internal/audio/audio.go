package audio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type RecordConfig struct {
	FFmpegPath      string
	Format          string
	Device          string
	OutputDir       string
	DurationSeconds int
}

type Recorder struct {
	cmd        *exec.Cmd
	stderr     bytes.Buffer
	OutputPath string
	stopped    bool
}

func StartRecording(ctx context.Context, config RecordConfig) (*Recorder, error) {
	if config.DurationSeconds <= 0 {
		config.DurationSeconds = 10
	}
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return nil, err
	}
	outputPath := filepath.Join(config.OutputDir, fmt.Sprintf("recording-%d.wav", time.Now().UnixNano()))

	args := []string{
		"-y",
		"-f", config.Format,
		"-i", config.Device,
		"-t", fmt.Sprintf("%d", config.DurationSeconds),
		"-ac", "1",
		"-ar", "44100",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, config.FFmpegPath, args...)

	recorder := &Recorder{cmd: cmd, OutputPath: outputPath}
	cmd.Stdout = &recorder.stderr
	cmd.Stderr = &recorder.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg recording failed: %w", err)
	}

	return recorder, nil
}

func (recorder *Recorder) Stop() error {
	if recorder == nil || recorder.cmd == nil || recorder.cmd.Process == nil {
		return nil
	}
	recorder.stopped = true
	if err := recorder.cmd.Process.Signal(os.Interrupt); err != nil {
		_ = recorder.cmd.Process.Kill()
		return err
	}
	return nil
}

func (recorder *Recorder) Wait() error {
	if recorder == nil || recorder.cmd == nil {
		return nil
	}
	if err := recorder.cmd.Wait(); err != nil {
		if recorder.stopped {
			return nil
		}
		return fmt.Errorf("ffmpeg recording failed: %s", strings.TrimSpace(recorder.stderr.String()))
	}
	return nil
}

func ValidateAudioPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("audio path is a directory")
	}
	return nil
}

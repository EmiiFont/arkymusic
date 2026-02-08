package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/audio2videoAI/internal/ai/elevenlabs"
	"github.com/audio2videoAI/internal/ai/replicate"
	"github.com/audio2videoAI/internal/audio"
)

type Event struct {
	Stage          string
	Message        string
	Progress       float64
	Transcript     string
	TranscriptPath string
}

type JobInput struct {
	AudioPath       string
	Lyrics          string
	Preset          string
	StylePreset     string
	AspectRatio     string
	DurationSeconds int
	OutputDir       string
}

type Result struct {
	JobID     string
	VideoPath string
	MetaPath  string
}

type Runner struct {
	ElevenLabs   *elevenlabs.Client
	Replicate    *replicate.Client
	Transcribe   audio.TranscribeConfig
	FFmpegPath   string
	PollInterval time.Duration
	PreferWait   bool
}

func (runner *Runner) Run(ctx context.Context, input JobInput, events chan<- Event) (Result, error) {
	send := func(stage, message string, progress float64) {
		if events != nil {
			events <- Event{Stage: stage, Message: message, Progress: progress}
		}
	}

	sendTranscript := func(stage, message, transcript, transcriptPath string, progress float64) {
		if events != nil {
			events <- Event{
				Stage:          stage,
				Message:        message,
				Progress:       progress,
				Transcript:     transcript,
				TranscriptPath: transcriptPath,
			}
		}
	}

	if runner.Replicate == nil {
		return Result{}, fmt.Errorf("replicate client not configured")
	}

	send("validate", "Validating audio", 0.05)
	if err := audio.ValidateAudioPath(input.AudioPath); err != nil {
		return Result{}, err
	}

	send("enhance", "Enhancing audio", 0.2)
	enhancedPath := input.AudioPath
	if runner.ElevenLabs != nil {
		var err error
		enhancedPath, err = runner.ElevenLabs.EnhanceAudio(ctx, input.AudioPath, input.OutputDir)
		if err != nil {
			return Result{}, err
		}
	}

	var transcript string
	var transcriptPath string
	if runner.Transcribe.Enabled {
		send("transcribe", "Transcribing audio", 0.3)
		var err error
		transcript, transcriptPath, err = audio.Transcribe(ctx, runner.Transcribe, input.AudioPath, input.OutputDir)
		if err != nil {
			return Result{}, err
		}
		sendTranscript("transcribe", "Transcript ready", transcript, transcriptPath, 0.35)
	}

	send("analyze", "Analyzing audio", 0.36)
	analysis, err := audio.Analyze(ctx, runner.FFmpegPath, input.AudioPath)
	if err != nil {
		return Result{}, err
	}

	send("submit", "Submitting to Replicate", 0.4)
	prompt := buildPrompt(input, enhancedPath, transcript, analysis)
	prediction, err := runner.Replicate.SubmitPrediction(ctx, replicate.PredictionRequest{
		Input: map[string]any{
			"prompt":           prompt,
			"prompt_optimizer": true,
			"duration":         input.DurationSeconds,
			"aspect_ratio":     input.AspectRatio,
		},
	}, runner.PreferWait)
	if err != nil {
		return Result{}, err
	}

	prediction, err = runner.pollPrediction(ctx, prediction, send)
	if err != nil {
		return Result{}, err
	}

	send("download", "Downloading video", 0.9)
	videoURL := replicate.OutputURL(prediction.Output)
	if videoURL == "" {
		return Result{}, fmt.Errorf("replicate output url missing")
	}
	videoPath, err := downloadOutput(ctx, videoURL, input.OutputDir)
	if err != nil {
		return Result{}, err
	}

	muxedPath, err := muxAudio(ctx, runner.FFmpegPath, videoPath, input.AudioPath, input.OutputDir)
	if err != nil {
		return Result{}, err
	}

	metaPath, err := writeMetadata(input, prediction.ID, muxedPath, transcript, transcriptPath, analysis)
	if err != nil {
		return Result{}, err
	}

	send("done", "Completed", 1.0)
	return Result{JobID: prediction.ID, VideoPath: videoPath, MetaPath: metaPath}, nil
}

func (runner *Runner) pollPrediction(ctx context.Context, prediction replicate.Prediction, send func(string, string, float64)) (replicate.Prediction, error) {
	pollInterval := runner.PollInterval
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}

	for {
		status := strings.ToLower(prediction.Status)
		progress := 0.4
		if status == "processing" || status == "running" {
			progress = 0.6
		}
		send("render", fmt.Sprintf("Rendering (%s)", prediction.Status), progress)

		switch status {
		case "succeeded", "completed":
			return prediction, nil
		case "failed", "canceled":
			return replicate.Prediction{}, fmt.Errorf("replicate job failed")
		case "starting", "processing", "running", "queued":
			// continue polling
		default:
			return replicate.Prediction{}, fmt.Errorf("replicate job status unknown: %s", prediction.Status)
		}

		select {
		case <-ctx.Done():
			return replicate.Prediction{}, ctx.Err()
		case <-time.After(pollInterval):
		}

		var err error
		prediction, err = runner.Replicate.FetchPrediction(ctx, prediction.ID)
		if err != nil {
			return replicate.Prediction{}, err
		}
	}
}

func muxAudio(ctx context.Context, ffmpegPath, videoPath, audioPath, outputDir string) (string, error) {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	outputPath := filepath.Join(outputDir, fmt.Sprintf("final-%d.mp4", time.Now().UnixNano()))

	cmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-shortest",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg mux failed: %s", strings.TrimSpace(string(output)))
	}
	return outputPath, nil
}

func downloadOutput(ctx context.Context, url, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("replicate download error: %s", string(body))
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("video-%d.mp4", time.Now().UnixNano()))
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer outputFile.Close()

	if _, err := io.Copy(outputFile, response.Body); err != nil {
		return "", err
	}
	return outputPath, nil
}

func buildPrompt(input JobInput, enhancedPath, transcript string, analysis audio.Analysis) string {
	promptParts := []string{"cinematic music video visuals"}
	promptParts = append(promptParts, presetNotes(input.Preset)...)
	if input.StylePreset != "" {
		promptParts = append(promptParts, fmt.Sprintf("style %s", input.StylePreset))
	}
	if input.AspectRatio != "" {
		promptParts = append(promptParts, fmt.Sprintf("aspect ratio %s", input.AspectRatio))
	}
	if strings.TrimSpace(input.Lyrics) != "" {
		promptParts = append(promptParts, fmt.Sprintf("lyrics: %s", strings.TrimSpace(input.Lyrics)))
	}
	if strings.TrimSpace(transcript) != "" {
		promptParts = append(promptParts, fmt.Sprintf("transcript: %s", strings.TrimSpace(transcript)))
	}
	promptParts = append(promptParts, vibeFromAnalysis(analysis)...)
	if enhancedPath != "" {
		promptParts = append(promptParts, fmt.Sprintf("audio source %s", filepath.Base(enhancedPath)))
	}
	return strings.Join(promptParts, ", ")
}

func presetNotes(preset string) []string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "hook":
		return []string{"hook moment", "fast impact", "center focus"}
	case "canvas":
		return []string{"seamless loop", "subtle motion", "ambient visuals"}
	case "highlight":
		return []string{"dramatic highlight", "cinematic focus", "story beat"}
	default:
		return nil
	}
}

func vibeFromAnalysis(analysis audio.Analysis) []string {
	var notes []string
	if analysis.BPM >= 120 {
		notes = append(notes, "fast paced", "dynamic cuts", "high energy")
	} else if analysis.BPM > 0 && analysis.BPM <= 90 {
		notes = append(notes, "slow motion", "smooth transitions", "ambient")
	}
	if analysis.MaxVolume >= -10 {
		notes = append(notes, "intense", "vibrant colors", "high contrast")
	} else if analysis.MeanVolume <= -25 && analysis.MeanVolume < 0 {
		notes = append(notes, "minimalist", "soft lighting", "calm")
	}
	return notes
}

func writeMetadata(input JobInput, jobID, videoPath, transcript, transcriptPath string, analysis audio.Analysis) (string, error) {
	if err := os.MkdirAll(input.OutputDir, 0o755); err != nil {
		return "", err
	}

	payload := map[string]any{
		"job_id":           jobID,
		"audio_path":       input.AudioPath,
		"lyrics":           input.Lyrics,
		"preset":           input.Preset,
		"style_preset":     input.StylePreset,
		"aspect_ratio":     input.AspectRatio,
		"duration_seconds": input.DurationSeconds,
		"video_path":       videoPath,
		"transcript":       transcript,
		"transcript_path":  transcriptPath,
		"audio_bpm":        analysis.BPM,
		"audio_mean_db":    analysis.MeanVolume,
		"audio_max_db":     analysis.MaxVolume,
		"audio_duration":   analysis.Duration,
		"created_at":       time.Now().Format(time.RFC3339),
	}

	metaPath := filepath.Join(input.OutputDir, fmt.Sprintf("metadata-%d.json", time.Now().UnixNano()))
	file, err := os.Create(metaPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(payload); err != nil {
		return "", err
	}
	return metaPath, nil
}

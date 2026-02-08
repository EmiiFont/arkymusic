package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Analysis struct {
	BPM        float64
	MeanVolume float64
	MaxVolume  float64
	Duration   float64
}

func Analyze(ctx context.Context, ffmpegPath, inputPath string) (Analysis, error) {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	cmdVol := exec.CommandContext(ctx, ffmpegPath, "-i", inputPath, "-filter:a", "volumedetect", "-f", "null", "/dev/null")
	var stderrVol bytes.Buffer
	cmdVol.Stderr = &stderrVol
	if err := cmdVol.Run(); err != nil {
		return Analysis{}, fmt.Errorf("volume analysis failed: %w", err)
	}
	outputVol := stderrVol.String()

	meanVol := parseFFmpegValue(outputVol, "mean_volume: ", " dB")
	maxVol := parseFFmpegValue(outputVol, "max_volume: ", " dB")
	duration := parseDurationFromLog(outputVol)

	bpm := 0.0
	cmdBpm := exec.CommandContext(ctx, ffmpegPath, "-i", inputPath, "-filter:a", "bpm", "-f", "null", "/dev/null")
	var stderrBpm bytes.Buffer
	cmdBpm.Stderr = &stderrBpm
	if err := cmdBpm.Run(); err == nil {
		bpm = parseBPM(stderrBpm.String())
	}

	return Analysis{
		BPM:        bpm,
		MeanVolume: meanVol,
		MaxVolume:  maxVol,
		Duration:   duration,
	}, nil
}

func parseFFmpegValue(output, prefix, suffix string) float64 {
	idx := strings.Index(output, prefix)
	if idx == -1 {
		return 0.0
	}
	rest := output[idx+len(prefix):]
	endIdx := strings.Index(rest, suffix)
	if endIdx == -1 {
		return 0.0
	}
	valStr := strings.TrimSpace(rest[:endIdx])
	val, _ := strconv.ParseFloat(valStr, 64)
	return val
}

func parseDurationFromLog(output string) float64 {
	re := regexp.MustCompile(`Duration: (\d{2}):(\d{2}):(\d{2}\.\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) == 4 {
		h, _ := strconv.ParseFloat(matches[1], 64)
		m, _ := strconv.ParseFloat(matches[2], 64)
		s, _ := strconv.ParseFloat(matches[3], 64)
		return h*3600 + m*60 + s
	}
	return 0.0
}

func parseBPM(output string) float64 {
	lines := strings.Split(output, "\n")
	var totalBPM float64
	var count int

	for _, line := range lines {
		if idx := strings.Index(line, "BPM: "); idx != -1 {
			valStr := strings.TrimSpace(line[idx+5:])
			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil && val > 0 {
				totalBPM += val
				count++
			}
		}
	}
	if count > 0 {
		return totalBPM / float64(count)
	}
	return 0.0
}

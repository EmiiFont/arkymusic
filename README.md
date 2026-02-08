# Audio2VideoAI TUI

Generate cinematic short-form videos from audio using a local TUI. The MVP flow supports local audio files, recording from an input device, optional lyrics, and Replicate rendering with progress updates.

## Requirements

- Go 1.25+
- Docker (for Whisper transcription)
- `ffmpeg` available on PATH (or set `FFMPEG_PATH`)
- Replicate API key (for video generation), loaded from `.env` if present
- ElevenLabs API key (optional, for enhancement)

## Run

```bash
go mod tidy

# First-time Whisper model download (small)
mkdir -p ./models

docker run --rm -v ./models:/models ghcr.io/ggml-org/whisper.cpp:main \
  ./models/download-ggml-model.sh small

# Option 1: set env directly
REPLICATE_API_TOKEN=... ELEVENLABS_API_KEY=... go run ./cmd/a2v

# Option 2: load from .env
cp .env.example .env
# edit .env then

go run ./cmd/a2v
```

## TUI Flow

1. Choose input type (audio file or record).
2. Optional lyrics entry.
3. Select style preset, aspect ratio, and duration.
4. Run generation and monitor progress.
5. Output saved to `./outputs`.

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `ELEVENLABS_API_KEY` | empty | Enables ElevenLabs enhancement when set. |
| `ELEVENLABS_BASE_URL` | `https://api.elevenlabs.io` | Base URL for ElevenLabs. |
| `ELEVENLABS_ENHANCE_PATH` | `/v1/audio-isolation` | Enhancement endpoint path. |
| `REPLICATE_API_TOKEN` | empty | API token for Replicate (required). |
| `REPLICATE_BASE_URL` | `https://api.replicate.com/v1` | Replicate API base URL. |
| `REPLICATE_MODEL` | `minimax/video-01` | Replicate model name. |
| `REPLICATE_PREFER_WAIT` | `true` | Wait for job completion in submit call. |
| `TRANSCRIBE_ENABLED` | `true` | Enable Whisper transcription. |
| `WHISPER_DOCKER_PATH` | `docker` | Docker CLI path. |
| `WHISPER_DOCKER_IMAGE` | `ghcr.io/ggml-org/whisper.cpp:main` | Whisper container image. |
| `WHISPER_MODEL` | `small` | Whisper model name. |
| `WHISPER_MODEL_DIR` | `./models` | Local model cache directory. |
| `WHISPER_AUTO_DOWNLOAD` | `true` | Auto-download model if missing. |
| `OUTPUT_DIR` | `./outputs` | Output directory for generated videos. |
| `FFMPEG_PATH` | `ffmpeg` | Path to `ffmpeg`. |
| `AUDIO_RECORD_FORMAT` | `alsa` | Recording input format for `ffmpeg`. |
| `AUDIO_RECORD_DEVICE` | `default` | Recording device. |
| `AUDIO_RECORD_SECONDS` | `15` | Default recording duration in seconds. |
| `JOB_POLL_INTERVAL` | `4s` | Replicate polling interval. |
| `HTTP_TIMEOUT` | `5m` | HTTP timeout for API calls. |

## Outputs

Each run writes:

- `final-*.mp4` generated output with original audio
- `video-*.mp4` downloaded video (before audio mux)
- `metadata-*.json` containing run configuration
- `transcript-*.txt` Whisper transcript
- `enhanced-*.wav` if ElevenLabs enhancement is enabled
- `recording-*.wav` if recording from input device

## Notes

- Recording is currently wired for ALSA (`AUDIO_RECORD_FORMAT=alsa`). Override for macOS/Windows as needed.
- During recording, press Space to stop early (max duration uses `AUDIO_RECORD_SECONDS`).
- Lyrics are optional and can be skipped with Enter or Ctrl+S.
- Replicate uses a prompt built from style + lyrics + full transcript.
- Download a Whisper model once, then reuse it across runs.
- Whisper transcription runs in Docker; disable with `TRANSCRIBE_ENABLED=false`.

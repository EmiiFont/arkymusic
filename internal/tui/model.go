package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/audio2videoAI/internal/audio"
	"github.com/audio2videoAI/internal/jobs"
	"github.com/audio2videoAI/pkg/config"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Step int

const (
	stepInputType Step = iota
	stepAudioPath
	stepRecordSettings
	stepLyrics
	stepStyle
	stepAspect
	stepDuration
	stepConfirm
	stepRunning
	stepDone
)

type inputType int

const (
	inputAudioFile inputType = iota
	inputRecord
)

type jobStartedMsg struct {
	events <-chan jobs.Event
	done   <-chan jobFinishedMsg
}

type jobFinishedMsg struct {
	result jobs.Result
	err    error
}

type jobProgressMsg jobs.Event

type recordStartedMsg struct {
	recorder *audio.Recorder
	err      error
}

type recordFinishedMsg struct {
	path string
	err  error
}

type recordTickMsg struct{}

type Model struct {
	config config.Config
	runner *jobs.Runner

	step             Step
	inputType        inputType
	inputTypeIdx     int
	styleIdx         int
	aspectIdx        int
	audioPath        string
	lyrics           string
	status           string
	err              error
	result           *jobs.Result
	transcript       string
	transcriptPath   string
	recording        bool
	recorder         *audio.Recorder
	recordingStart   time.Time
	recordingElapsed int
	jobRunning       bool
	jobEvents        []jobs.Event
	jobEventIndex    int

	audioPathInput    textinput.Model
	recordDeviceInput textinput.Model
	recordDurationInp textinput.Model
	durationInput     textinput.Model
	lyricsInput       textarea.Model

	progress progress.Model
	spinner  spinner.Model

	eventChan <-chan jobs.Event
	doneChan  <-chan jobFinishedMsg
}

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	highlight    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	subtle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	quitHint     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func NewModel(cfg config.Config, runner *jobs.Runner) Model {
	audioPathInput := textinput.New()
	audioPathInput.Placeholder = "/path/to/audio.wav"
	audioPathInput.Focus()

	recordDeviceInput := textinput.New()
	recordDeviceInput.Placeholder = cfg.RecordDevice
	if recordDeviceInput.Placeholder == "" {
		recordDeviceInput.Placeholder = "default"
	}

	recordDurationInput := textinput.New()
	recordDurationInput.Placeholder = fmt.Sprintf("%d", cfg.RecordDurationSeconds)

	durationInput := textinput.New()
	durationInput.Placeholder = "30"
	durationInput.SetValue("30")

	lyricsInput := textarea.New()
	lyricsInput.Placeholder = "Optional lyrics (press Ctrl+S to continue)"
	lyricsInput.ShowLineNumbers = false
	lyricsInput.SetWidth(60)
	lyricsInput.SetHeight(6)

	progressBar := progress.New(progress.WithDefaultGradient())

	spinnerModel := spinner.New()
	spinnerModel.Spinner = spinner.Dot

	return Model{
		config:            cfg,
		runner:            runner,
		step:              stepInputType,
		inputTypeIdx:      0,
		styleIdx:          0,
		aspectIdx:         0,
		audioPathInput:    audioPathInput,
		recordDeviceInput: recordDeviceInput,
		recordDurationInp: recordDurationInput,
		durationInput:     durationInput,
		lyricsInput:       lyricsInput,
		progress:          progressBar,
		spinner:           spinnerModel,
	}
}

func (model Model) Init() tea.Cmd {
	return model.spinner.Tick
}

func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		model.spinner, cmd = model.spinner.Update(msg)
		return model, cmd
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return model, tea.Quit
		}
		return model.handleKey(msg)
	case recordStartedMsg:
		if msg.err != nil || msg.recorder == nil {
			if msg.err != nil {
				model.err = msg.err
			} else {
				model.err = fmt.Errorf("recorder not initialized")
			}
			model.status = "Recording failed"
			model.recording = false
			return model, nil
		}
		model.recorder = msg.recorder
		model.recording = true
		model.recordingStart = time.Now()
		model.recordingElapsed = 0
		model.status = "Recording audio"
		return model, tea.Batch(recordTickCmd(), recordWaitCmd(model.recorder))
	case recordTickMsg:
		if model.recording {
			model.recordingElapsed = int(time.Since(model.recordingStart).Seconds())
			return model, recordTickCmd()
		}
		return model, nil
	case recordFinishedMsg:
		model.recording = false
		model.recorder = nil
		if msg.err != nil {
			model.err = msg.err
			model.status = "Recording failed"
			return model, nil
		}
		model.audioPath = msg.path
		model.step = stepLyrics
		model.status = "Recording complete"
		model.lyricsInput.Focus()
		return model, nil
	case jobStartedMsg:
		model.eventChan = msg.events
		model.doneChan = msg.done
		model.jobRunning = true
		return model, tea.Batch(listenEventCmd(model.eventChan), listenDoneCmd(model.doneChan))
	case jobProgressMsg:
		model.jobEvents = append(model.jobEvents, jobs.Event(msg))
		if len(model.jobEvents) > 6 {
			model.jobEvents = model.jobEvents[len(model.jobEvents)-6:]
		}
		if msg.Transcript != "" {
			model.transcript = msg.Transcript
			model.transcriptPath = msg.TranscriptPath
		}
		model.status = msg.Message
		model.progress.SetPercent(msg.Progress)
		return model, listenEventCmd(model.eventChan)
	case jobFinishedMsg:
		model.jobRunning = false
		if msg.err != nil {
			model.err = msg.err
			model.status = "Job failed"
			model.step = stepDone
			return model, nil
		}
		model.result = &msg.result
		model.status = "Video ready"
		model.step = stepDone
		return model, nil
	case tea.WindowSizeMsg:
		model.progress.Width = msg.Width - 8
		return model, nil
	}

	return model, nil
}

func (model Model) View() string {
	var view string
	switch model.step {
	case stepInputType:
		view = model.viewInputType()
	case stepAudioPath:
		view = model.viewAudioPath()
	case stepRecordSettings:
		view = model.viewRecord()
	case stepLyrics:
		view = model.viewLyrics()
	case stepStyle:
		view = model.viewStyle()
	case stepAspect:
		view = model.viewAspect()
	case stepDuration:
		view = model.viewDuration()
	case stepConfirm:
		view = model.viewConfirm()
	case stepRunning:
		view = model.viewRunning()
	case stepDone:
		view = model.viewDone()
	default:
		view = ""
	}
	return view + "\n\n" + quitHint.Render("Press q or Ctrl+C to quit")
}

func (model Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch model.step {
	case stepInputType:
		switch msg.String() {
		case "up", "k":
			model.inputTypeIdx = (model.inputTypeIdx + len(inputOptions()) - 1) % len(inputOptions())
		case "down", "j":
			model.inputTypeIdx = (model.inputTypeIdx + 1) % len(inputOptions())
		case "enter":
			if model.inputTypeIdx == 0 {
				model.inputType = inputAudioFile
				model.step = stepAudioPath
				model.audioPathInput.Focus()
			} else {
				model.inputType = inputRecord
				model.step = stepRecordSettings
				model.recordDeviceInput.Focus()
			}
		}
	case stepAudioPath:
		var cmd tea.Cmd
		model.audioPathInput, cmd = model.audioPathInput.Update(msg)
		switch msg.String() {
		case "enter":
			model.audioPath = strings.TrimSpace(model.audioPathInput.Value())
			if model.audioPath != "" {
				model.step = stepLyrics
				model.lyricsInput.Focus()
			}
		}
		return model, cmd
	case stepRecordSettings:
		if model.recording {
			switch msg.String() {
			case " ":
				if model.recorder != nil {
					if err := model.recorder.Stop(); err != nil {
						model.err = err
						model.status = "Recording stop failed"
						return model, nil
					}
					model.status = "Stopping recording"
				}
			}
			return model, nil
		}

		var cmd tea.Cmd
		model.recordDeviceInput, cmd = model.recordDeviceInput.Update(msg)
		model.recordDurationInp, _ = model.recordDurationInp.Update(msg)
		switch msg.String() {
		case "enter":
			return model, startRecordCmd(model.config, model.recordDeviceInput.Value(), model.recordDurationInp.Value())
		}
		return model, cmd
	case stepLyrics:
		var cmd tea.Cmd
		model.lyricsInput, cmd = model.lyricsInput.Update(msg)
		if msg.Type == tea.KeyCtrlS || msg.String() == "enter" {
			model.lyrics = strings.TrimSpace(model.lyricsInput.Value())
			model.step = stepStyle
		}
		return model, cmd
	case stepStyle:
		switch msg.String() {
		case "up", "k":
			model.styleIdx = (model.styleIdx + len(styleOptions()) - 1) % len(styleOptions())
		case "down", "j":
			model.styleIdx = (model.styleIdx + 1) % len(styleOptions())
		case "enter":
			model.step = stepAspect
		}
	case stepAspect:
		switch msg.String() {
		case "up", "k":
			model.aspectIdx = (model.aspectIdx + len(aspectOptions()) - 1) % len(aspectOptions())
		case "down", "j":
			model.aspectIdx = (model.aspectIdx + 1) % len(aspectOptions())
		case "enter":
			model.step = stepDuration
			model.durationInput.Focus()
		}
	case stepDuration:
		var cmd tea.Cmd
		model.durationInput, cmd = model.durationInput.Update(msg)
		switch msg.String() {
		case "enter":
			model.step = stepConfirm
		}
		return model, cmd
	case stepConfirm:
		switch msg.String() {
		case "enter":
			model.step = stepRunning
			return model, model.startJobCmd()
		case "esc":
			model.step = stepDuration
		}
	case stepDone:
		switch msg.String() {
		case "q", "ctrl+c":
			return model, tea.Quit
		}
	}

	return model, nil
}

func (model Model) viewInputType() string {
	return renderSelect("Choose input type", inputOptions(), model.inputTypeIdx)
}

func (model Model) viewAudioPath() string {
	return fmt.Sprintf("%s\n\nAudio file path:\n%s\n\n%s", headerStyle.Render("Audio File"), model.audioPathInput.View(), subtle.Render("Press Enter to continue"))
}

func (model Model) viewRecord() string {
	if model.recording {
		maxSeconds := model.recordMaxDuration()
		return fmt.Sprintf(
			"%s\n\n%s Recording... %ds\n\n%s",
			headerStyle.Render("Recording"),
			model.spinner.View(),
			model.recordingElapsed,
			subtle.Render(fmt.Sprintf("Press Space to stop (max %ds)", maxSeconds)),
		)
	}
	return fmt.Sprintf("%s\n\nDevice:\n%s\n\nDuration (seconds):\n%s\n\n%s", headerStyle.Render("Record Audio"), model.recordDeviceInput.View(), model.recordDurationInp.View(), subtle.Render("Press Enter to start recording"))
}

func (model Model) viewLyrics() string {
	return fmt.Sprintf("%s\n\n%s\n\n%s", headerStyle.Render("Lyrics (optional)"), model.lyricsInput.View(), subtle.Render("Ctrl+S or Enter to continue"))
}

func (model Model) viewStyle() string {
	return renderSelect("Select style preset", styleOptions(), model.styleIdx)
}

func (model Model) viewAspect() string {
	return renderSelect("Select aspect ratio", aspectOptions(), model.aspectIdx)
}

func (model Model) viewDuration() string {
	return fmt.Sprintf("%s\n\nDuration (seconds):\n%s\n\n%s", headerStyle.Render("Duration"), model.durationInput.View(), subtle.Render("Press Enter to continue"))
}

func (model Model) viewConfirm() string {
	return fmt.Sprintf(
		"%s\n\nAudio: %s\nStyle: %s\nAspect: %s\nDuration: %s\nLyrics: %s\n\n%s",
		headerStyle.Render("Confirm"),
		model.audioPath,
		styleOptions()[model.styleIdx],
		aspectOptions()[model.aspectIdx],
		model.durationInput.Value(),
		lyricsSummary(model.lyrics),
		subtle.Render("Press Enter to start, Esc to edit"),
	)
}

func (model Model) viewRunning() string {
	lines := []string{headerStyle.Render("Generating video"), ""}
	if model.status != "" {
		lines = append(lines, statusStyle.Render(model.status))
	}
	lines = append(lines, model.progress.View())
	if model.transcript != "" {
		lines = append(lines, "", subtle.Render("Transcript preview:"), truncateText(model.transcript, 280))
		if model.transcriptPath != "" {
			lines = append(lines, subtle.Render("Saved: "+model.transcriptPath))
		}
	}
	if len(model.jobEvents) > 0 {
		lines = append(lines, "", subtle.Render("Recent events:"))
		for _, event := range model.jobEvents {
			lines = append(lines, fmt.Sprintf("- %s: %s", event.Stage, event.Message))
		}
	}
	return strings.Join(lines, "\n")
}

func (model Model) viewDone() string {
	if model.err != nil {
		return fmt.Sprintf("%s\n\n%s\n\n%s", headerStyle.Render("Error"), warningStyle.Render(model.err.Error()), subtle.Render("Press q to quit"))
	}
	if model.result == nil {
		return fmt.Sprintf("%s\n\n%s", headerStyle.Render("Done"), subtle.Render("Press q to quit"))
	}
	return fmt.Sprintf(
		"%s\n\nVideo: %s\nMetadata: %s\n\n%s",
		headerStyle.Render("Done"),
		model.result.VideoPath,
		model.result.MetaPath,
		subtle.Render("Press q to quit"),
	)
}

func (model Model) startJobCmd() tea.Cmd {
	input := jobs.JobInput{
		AudioPath:       model.audioPath,
		Lyrics:          model.lyrics,
		StylePreset:     styleOptions()[model.styleIdx],
		AspectRatio:     aspectOptions()[model.aspectIdx],
		DurationSeconds: parseDuration(model.durationInput.Value()),
		OutputDir:       model.config.OutputDir,
	}
	return func() tea.Msg {
		events := make(chan jobs.Event)
		done := make(chan jobFinishedMsg, 1)
		ctx := context.Background()
		go func() {
			result, err := model.runner.Run(ctx, input, events)
			close(events)
			done <- jobFinishedMsg{result: result, err: err}
		}()
		return jobStartedMsg{events: events, done: done}
	}
}

func startRecordCmd(cfg config.Config, deviceValue, durationValue string) tea.Cmd {
	return func() tea.Msg {
		duration := parseDuration(durationValue)
		if duration <= 0 {
			duration = cfg.RecordDurationSeconds
		}
		device := strings.TrimSpace(deviceValue)
		if device == "" {
			device = cfg.RecordDevice
		}
		recorder, err := audio.StartRecording(context.Background(), audio.RecordConfig{
			FFmpegPath:      cfg.FFmpegPath,
			Format:          cfg.RecordFormat,
			Device:          device,
			OutputDir:       cfg.OutputDir,
			DurationSeconds: duration,
		})
		return recordStartedMsg{recorder: recorder, err: err}
	}
}

func recordWaitCmd(recorder *audio.Recorder) tea.Cmd {
	return func() tea.Msg {
		if recorder == nil {
			return recordFinishedMsg{err: fmt.Errorf("recorder not initialized")}
		}
		err := recorder.Wait()
		return recordFinishedMsg{path: recorder.OutputPath, err: err}
	}
}

func recordTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return recordTickMsg{}
	})
}

func listenEventCmd(events <-chan jobs.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return jobProgressMsg(event)
	}
}

func listenDoneCmd(done <-chan jobFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		return <-done
	}
}

func (model Model) recordMaxDuration() int {
	value := parseDuration(model.recordDurationInp.Value())
	if value <= 0 {
		value = model.config.RecordDurationSeconds
	}
	if value <= 0 {
		value = 10
	}
	return value
}

func renderSelect(title string, options []string, selected int) string {
	lines := []string{headerStyle.Render(title), ""}
	for index, option := range options {
		cursor := "  "
		styled := option
		if index == selected {
			cursor = "> "
			styled = highlight.Render(option)
		}
		lines = append(lines, fmt.Sprintf("%s%s", cursor, styled))
	}
	lines = append(lines, "", subtle.Render("Use ↑/↓ and Enter"))
	return strings.Join(lines, "\n")
}

func inputOptions() []string {
	return []string{"Use audio file", "Record audio"}
}

func styleOptions() []string {
	return []string{"cinematic", "anime", "cyberpunk", "surreal", "minimalist"}
}

func aspectOptions() []string {
	return []string{"9:16", "1:1"}
}

func parseDuration(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var parsed int
	_, err := fmt.Sscanf(value, "%d", &parsed)
	if err != nil {
		return 0
	}
	return parsed
}

func lyricsSummary(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "(none)"
	}
	return truncateText(trimmed, 80)
}

func truncateText(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

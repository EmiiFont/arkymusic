package ltx2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Client struct {
	BaseURL      string
	GeneratePath string
	StatusPath   string
	DownloadPath string
	HTTPClient   *http.Client
}

type GenerateRequest struct {
	AudioPath       string
	Lyrics          string
	StylePreset     string
	AspectRatio     string
	DurationSeconds int
}

type JobStatus struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	OutputURL string  `json:"output_url"`
}

type GenerateResponse struct {
	JobID string `json:"job_id"`
	ID    string `json:"id"`
}

func NewClient(baseURL, generatePath, statusPath, downloadPath string, timeout time.Duration) *Client {
	return &Client{
		BaseURL:      baseURL,
		GeneratePath: generatePath,
		StatusPath:   statusPath,
		DownloadPath: downloadPath,
		HTTPClient:   &http.Client{Timeout: timeout},
	}
}

func (client *Client) SubmitJob(ctx context.Context, request GenerateRequest) (string, error) {
	if client.BaseURL == "" {
		return "", fmt.Errorf("ltx2 base url is required")
	}

	reqBody, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)

	requestURL := client.BaseURL + client.GeneratePath
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, reqBody)
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", multipartWriter.FormDataContentType())

	errorChan := make(chan error, 1)
	go func() {
		defer writer.Close()
		defer multipartWriter.Close()

		file, err := os.Open(request.AudioPath)
		if err != nil {
			errorChan <- err
			return
		}
		defer file.Close()

		part, err := multipartWriter.CreateFormFile("audio", filepath.Base(request.AudioPath))
		if err != nil {
			errorChan <- err
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			errorChan <- err
			return
		}

		_ = multipartWriter.WriteField("lyrics", request.Lyrics)
		_ = multipartWriter.WriteField("style", request.StylePreset)
		_ = multipartWriter.WriteField("aspect_ratio", request.AspectRatio)
		_ = multipartWriter.WriteField("duration_seconds", fmt.Sprintf("%d", request.DurationSeconds))

		errorChan <- nil
	}()

	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if err := <-errorChan; err != nil {
		return "", err
	}

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("ltx2 submit error: %s", string(body))
	}

	var payload GenerateResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}

	jobID := payload.JobID
	if jobID == "" {
		jobID = payload.ID
	}
	if jobID == "" {
		return "", fmt.Errorf("ltx2 response missing job id")
	}
	return jobID, nil
}

func (client *Client) FetchStatus(ctx context.Context, jobID string) (JobStatus, error) {
	url := client.BaseURL + fmt.Sprintf(client.StatusPath, jobID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return JobStatus{}, err
	}

	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return JobStatus{}, err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return JobStatus{}, fmt.Errorf("ltx2 status error: %s", string(body))
	}

	var status JobStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return JobStatus{}, err
	}
	if status.ID == "" {
		status.ID = jobID
	}
	return status, nil
}

func (client *Client) DownloadOutput(ctx context.Context, jobStatus JobStatus, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	downloadURL := jobStatus.OutputURL
	if downloadURL == "" {
		downloadURL = client.BaseURL + fmt.Sprintf(client.DownloadPath, jobStatus.ID)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("ltx2 download error: %s", string(body))
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

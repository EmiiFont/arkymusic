package elevenlabs

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Client struct {
	APIKey      string
	BaseURL     string
	EnhancePath string
	HTTPClient  *http.Client
}

func NewClient(apiKey, baseURL, enhancePath string, timeout time.Duration) *Client {
	return &Client{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		EnhancePath: enhancePath,
		HTTPClient:  &http.Client{Timeout: timeout},
	}
}

func (client *Client) EnhanceAudio(ctx context.Context, inputPath, outputDir string) (string, error) {
	if client.APIKey == "" {
		return inputPath, nil
	}

	reqBody, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+client.EnhancePath, reqBody)
	if err != nil {
		return "", err
	}

	request.Header.Set("xi-api-key", client.APIKey)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())

	errorChan := make(chan error, 1)
	go func() {
		defer writer.Close()
		defer multipartWriter.Close()

		file, err := os.Open(inputPath)
		if err != nil {
			errorChan <- err
			return
		}
		defer file.Close()

		part, err := multipartWriter.CreateFormFile("audio", filepath.Base(inputPath))
		if err != nil {
			errorChan <- err
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			errorChan <- err
			return
		}
		errorChan <- nil
	}()

	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if err := <-errorChan; err != nil {
		return "", err
	}

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("elevenlabs error: %s", string(body))
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	outputPath := filepath.Join(outputDir, fmt.Sprintf("enhanced-%d.wav", time.Now().UnixNano()))
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

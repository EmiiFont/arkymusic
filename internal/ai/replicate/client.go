package replicate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	APIToken   string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

type Prediction struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output any    `json:"output"`
	Error  any    `json:"error"`
	Logs   string `json:"logs"`
}

type PredictionRequest struct {
	Input map[string]any `json:"input"`
}

func NewClient(apiToken, baseURL, model string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = "https://api.replicate.com/v1"
	}
	if model == "" {
		model = "minimax/video-01"
	}
	return &Client{
		APIToken:   apiToken,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Model:      model,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (client *Client) SubmitPrediction(ctx context.Context, request PredictionRequest, preferWait bool) (Prediction, error) {
	if client.APIToken == "" {
		return Prediction{}, fmt.Errorf("replicate api token is required")
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return Prediction{}, err
	}

	url := fmt.Sprintf("%s/models/%s/predictions", client.BaseURL, client.Model)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return Prediction{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.APIToken)
	httpRequest.Header.Set("Content-Type", "application/json")
	if preferWait {
		httpRequest.Header.Set("Prefer", "wait")
	}

	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return Prediction{}, err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return Prediction{}, fmt.Errorf("replicate submit error: %s", string(body))
	}

	var prediction Prediction
	if err := json.NewDecoder(response.Body).Decode(&prediction); err != nil {
		return Prediction{}, err
	}
	return prediction, nil
}

func (client *Client) FetchPrediction(ctx context.Context, id string) (Prediction, error) {
	url := fmt.Sprintf("%s/predictions/%s", client.BaseURL, id)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Prediction{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.APIToken)

	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return Prediction{}, err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		return Prediction{}, fmt.Errorf("replicate status error: %s", string(body))
	}

	var prediction Prediction
	if err := json.NewDecoder(response.Body).Decode(&prediction); err != nil {
		return Prediction{}, err
	}
	return prediction, nil
}

func OutputURL(output any) string {
	switch value := output.(type) {
	case string:
		return value
	case []any:
		for _, item := range value {
			if url, ok := item.(string); ok {
				return url
			}
		}
	}
	return ""
}

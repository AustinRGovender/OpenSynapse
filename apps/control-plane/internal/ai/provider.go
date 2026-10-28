package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider is the interface for AI providers.
type Provider interface {
	Name() string
	Chat(apiKey, model, prompt string) (*ChatResponse, error)
	TestKey(apiKey, model string) error
}

// ChatResponse holds the result from an AI provider.
type ChatResponse struct {
	Content      string  `json:"content"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// GetProvider returns the provider implementation for the given name.
// endpoint is the Azure OpenAI resource URL; other providers ignore it.
func GetProvider(name, endpoint string) (Provider, error) {
	switch name {
	case "openai":
		return &OpenAIProvider{}, nil
	case "anthropic":
		return &AnthropicProvider{}, nil
	case "azure_openai":
		return &AzureOpenAIProvider{Endpoint: endpoint}, nil
	case "nvidia_nim":
		return &NIMProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// httpClient is shared by every provider. Hosted models can take well over
// 30 s to write a 500-word analysis, so the timeout is generous.
var httpClient = &http.Client{Timeout: 2 * time.Minute}

// --- OpenAI-compatible chat completions (OpenAI, Azure OpenAI, NVIDIA NIM) ---

// chatBody is a single-turn chat completions request. Azure routes by
// deployment name, so it passes an empty model and the field is left out.
func chatBody(model, prompt string) map[string]interface{} {
	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 2000,
	}
	if model != "" {
		body["model"] = model
	}
	return body
}

// chatCompletions posts body to an OpenAI-compatible endpoint and reads the
// first choice. label names the provider in errors; model prices the call.
func chatCompletions(label, url, model, authHeader, authValue string, body map[string]interface{}) (*ChatResponse, error) {
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authHeader, authValue)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", label, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s returned %d: %s", label, resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                   `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("%s returned an unreadable response: %w", label, err)
	}

	// Reasoning models can spend the whole token budget thinking and return
	// no answer; report that rather than caching an empty analysis.
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		reason := "none"
		if len(result.Choices) > 0 && result.Choices[0].FinishReason != "" {
			reason = result.Choices[0].FinishReason
		}
		return nil, fmt.Errorf("%s returned no answer (finish reason: %s)", label, reason)
	}

	cost := estimateCost(model, result.Usage.PromptTokens, result.Usage.CompletionTokens)

	return &ChatResponse{
		Content:      result.Choices[0].Message.Content,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		CostUSD:      cost,
	}, nil
}

// --- OpenAI ---

type OpenAIProvider struct{}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Chat(apiKey, model, prompt string) (*ChatResponse, error) {
	return chatCompletions("openai", "https://api.openai.com/v1/chat/completions", model,
		"Authorization", "Bearer "+apiKey, chatBody(model, prompt))
}

func (p *OpenAIProvider) TestKey(apiKey, model string) error {
	_, err := p.Chat(apiKey, model, "Say 'ok'")
	return err
}

// --- Anthropic ---

type AnthropicProvider struct{}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Chat(apiKey, model, prompt string) (*ChatResponse, error) {
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 2000,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct{ Text string } `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(respBody, &result)

	content := ""
	if len(result.Content) > 0 {
		content = result.Content[0].Text
	}

	cost := estimateCost(model, result.Usage.InputTokens, result.Usage.OutputTokens)

	return &ChatResponse{
		Content:      content,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		CostUSD:      cost,
	}, nil
}

func (p *AnthropicProvider) TestKey(apiKey, model string) error {
	_, err := p.Chat(apiKey, model, "Say 'ok'")
	return err
}

// --- Azure OpenAI ---

type AzureOpenAIProvider struct {
	Endpoint string
}

func (p *AzureOpenAIProvider) Name() string { return "azure_openai" }

func (p *AzureOpenAIProvider) Chat(apiKey, model, prompt string) (*ChatResponse, error) {
	if p.Endpoint == "" {
		return nil, fmt.Errorf("azure openai: no endpoint configured")
	}
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-01", strings.TrimRight(p.Endpoint, "/"), model)
	return chatCompletions("azure openai", url, model, "api-key", apiKey, chatBody("", prompt))
}

func (p *AzureOpenAIProvider) TestKey(apiKey, model string) error {
	_, err := p.Chat(apiKey, model, "Say 'ok'")
	return err
}

// --- NVIDIA NIM ---

// nimBaseURL is NVIDIA's hosted API catalog (build.nvidia.com).
const nimBaseURL = "https://integrate.api.nvidia.com/v1"

// NIMProvider calls NVIDIA NIM's OpenAI-compatible chat completions API.
// Model names are catalog IDs such as "z-ai/glm-5.3-flash".
type NIMProvider struct {
	BaseURL string // empty means NVIDIA's hosted API; a self-hosted NIM serves the same API
}

func (p *NIMProvider) Name() string { return "nvidia_nim" }

func (p *NIMProvider) Chat(apiKey, model, prompt string) (*ChatResponse, error) {
	base := p.BaseURL
	if base == "" {
		base = nimBaseURL
	}
	return chatCompletions("nvidia nim", strings.TrimRight(base, "/")+"/chat/completions", model,
		"Authorization", "Bearer "+apiKey, chatBody(model, prompt))
}

func (p *NIMProvider) TestKey(apiKey, model string) error {
	_, err := p.Chat(apiKey, model, "Say 'ok'")
	return err
}

// estimateCost provides rough cost estimates per model.
func estimateCost(model string, inputTokens, outputTokens int) float64 {
	// Approximate pricing per 1M tokens (input/output)
	prices := map[string][2]float64{
		"gpt-4o":                   {2.50, 10.00},
		"gpt-4-turbo":              {10.00, 30.00},
		"gpt-3.5-turbo":            {0.50, 1.50},
		"claude-sonnet-4-20250514": {3.00, 15.00},
		"claude-haiku-4-20250414":  {0.80, 4.00},
	}

	price, ok := prices[model]
	if !ok {
		price = [2]float64{5.00, 15.00} // default estimate
	}

	inputCost := float64(inputTokens) / 1_000_000.0 * price[0]
	outputCost := float64(outputTokens) / 1_000_000.0 * price[1]
	return inputCost + outputCost
}

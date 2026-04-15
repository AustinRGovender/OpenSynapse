package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func GetProvider(name string) (Provider, error) {
	switch name {
	case "openai":
		return &OpenAIProvider{}, nil
	case "anthropic":
		return &AnthropicProvider{}, nil
	case "azure_openai":
		return &AzureOpenAIProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// --- OpenAI ---

type OpenAIProvider struct{}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Chat(apiKey, model, prompt string) (*ChatResponse, error) {
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 2000,
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(respBody, &result)

	content := ""
	if len(result.Choices) > 0 {
		content = result.Choices[0].Message.Content
	}

	cost := estimateCost(model, result.Usage.PromptTokens, result.Usage.CompletionTokens)

	return &ChatResponse{
		Content:      content,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		CostUSD:      cost,
	}, nil
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

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-01", p.Endpoint, model)

	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 2000,
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure openai request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("azure openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(respBody, &result)

	content := ""
	if len(result.Choices) > 0 {
		content = result.Choices[0].Message.Content
	}

	cost := estimateCost(model, result.Usage.PromptTokens, result.Usage.CompletionTokens)

	return &ChatResponse{
		Content:      content,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		CostUSD:      cost,
	}, nil
}

func (p *AzureOpenAIProvider) TestKey(apiKey, model string) error {
	_, err := p.Chat(apiKey, model, "Say 'ok'")
	return err
}

// estimateCost provides rough cost estimates per model.
func estimateCost(model string, inputTokens, outputTokens int) float64 {
	// Approximate pricing per 1M tokens (input/output)
	prices := map[string][2]float64{
		"gpt-4o":          {2.50, 10.00},
		"gpt-4-turbo":     {10.00, 30.00},
		"gpt-3.5-turbo":   {0.50, 1.50},
		"claude-sonnet-4-20250514":  {3.00, 15.00},
		"claude-haiku-4-20250414": {0.80, 4.00},
	}

	price, ok := prices[model]
	if !ok {
		price = [2]float64{5.00, 15.00} // default estimate
	}

	inputCost := float64(inputTokens) / 1_000_000.0 * price[0]
	outputCost := float64(outputTokens) / 1_000_000.0 * price[1]
	return inputCost + outputCost
}

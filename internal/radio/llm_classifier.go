package radio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// LLMEngine defines the low-level model inference backend (e.g. Qwen2.5-0.5B via Ollama or ONNX).
type LLMEngine interface {
	Generate(ctx context.Context, prompt string) (string, error)
	Close() error
}

// OfflineLLMEngine provides a deterministic offline engine for CI, testing, and zero-network setups.
type OfflineLLMEngine struct {
	mockResponse string
	fallback     *RuleClassifier
}

// NewOfflineLLMEngine creates a new OfflineLLMEngine.
func NewOfflineLLMEngine() *OfflineLLMEngine {
	return &OfflineLLMEngine{
		fallback: NewRuleClassifier(),
	}
}

// SetMockResponse sets an explicit JSON string response for testing.
func (e *OfflineLLMEngine) SetMockResponse(resp string) {
	e.mockResponse = resp
}

// Generate returns the mock response or an empty JSON object.
func (e *OfflineLLMEngine) Generate(ctx context.Context, prompt string) (string, error) {
	if e.mockResponse != "" {
		return e.mockResponse, nil
	}
	return "{}", nil
}

// Close releases resources.
func (e *OfflineLLMEngine) Close() error {
	if e.fallback != nil {
		return e.fallback.Close()
	}
	return nil
}

// DefaultLLMEndpoint is the dedicated loopback inference port for dualsense-c.
// Project port allocation table:
//   - 9999: UDP Bridge (Assetto Corsa / ACC / ACR)
//   - 9998: IPC Force Feedback (FFB1)
//   - 9997: IPC Telemetry Broadcast (SIG1)
//   - 9995: Companion Local SLM / LLM Inference (HTTPLLMEngine)
const DefaultLLMEndpoint = "http://127.0.0.1:9995/completion"

// ResolveLLMEndpoint resolves the LLM inference URL from explicit argument,
// environment variables (DS5_LLM_ENDPOINT / DUALSENSE_LLM_ENDPOINT), or the project default.
func ResolveLLMEndpoint(custom string) string {
	if custom != "" {
		return normalizeEndpoint(custom)
	}
	if env := os.Getenv("DS5_LLM_ENDPOINT"); env != "" {
		return normalizeEndpoint(env)
	}
	if env := os.Getenv("DUALSENSE_LLM_ENDPOINT"); env != "" {
		return normalizeEndpoint(env)
	}
	return DefaultLLMEndpoint
}

func normalizeEndpoint(url string) string {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	// If only host:port provided without path, default to /completion
	if !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://"), "/") {
		url = strings.TrimRight(url, "/") + "/completion"
	}
	return url
}

// HTTPLLMEngine connects to an OpenAI/Ollama/llama-server compatible HTTP inference endpoint.
type HTTPLLMEngine struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewHTTPLLMEngine creates a new HTTPLLMEngine pointing to endpoint with model.
func NewHTTPLLMEngine(endpoint, model string) *HTTPLLMEngine {
	resolved := ResolveLLMEndpoint(endpoint)
	if model == "" {
		if env := os.Getenv("DS5_LLM_MODEL"); env != "" {
			model = env
		} else {
			model = "qwen2.5:0.5b"
		}
	}
	tr := &http.Transport{
		Proxy: nil, // direct loopback connection, immune to proxy misconfiguration
	}
	return &HTTPLLMEngine{
		endpoint: resolved,
		model:    model,
		client:   &http.Client{Timeout: 1500 * time.Millisecond, Transport: tr},
	}
}

// Generate issues a completion request to the inference endpoint.
func (e *HTTPLLMEngine) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]any{
		"model":     e.model,
		"prompt":    prompt,
		"stream":    false,
		"n_predict": 48,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http llm error: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Response string `json:"response"`
		Content  string `json:"content"`
		Choices  []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Content != "" {
			return parsed.Content, nil
		}
		if parsed.Response != "" {
			return parsed.Response, nil
		}
		if len(parsed.Choices) > 0 && parsed.Choices[0].Message.Content != "" {
			return parsed.Choices[0].Message.Content, nil
		}
	}
	return string(body), nil
}

// Close closes idle HTTP connections.
func (e *HTTPLLMEngine) Close() error {
	if e.client != nil {
		e.client.CloseIdleConnections()
	}
	return nil
}

// LLMClassifier wraps an LLMEngine and sliding telemetry window with Persona guidance.
type LLMClassifier struct {
	mu       sync.RWMutex
	engine   LLMEngine
	persona  Persona
	fallback *RuleClassifier
	logger   *log.Logger
}

// NewLLMClassifier creates a new LLMClassifier.
func NewLLMClassifier(engine LLMEngine, persona Persona, logger *log.Logger) *LLMClassifier {
	if logger == nil {
		logger = log.Default()
	}
	return &LLMClassifier{
		engine:   engine,
		persona:  persona,
		fallback: NewRuleClassifier(),
		logger:   logger,
	}
}

// SetPersona updates the active persona dynamically.
func (c *LLMClassifier) SetPersona(p Persona) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.persona = p
}

// Classify formats the telemetry window into Persona ChatML prompt, queries the LLM,
// and parses structured RadioIntent decisions with graceful rule fallback.
func (c *LLMClassifier) Classify(ctx context.Context, w *TelemetryWindow) (*RadioIntent, error) {
	if w == nil || w.Count() == 0 {
		return nil, nil
	}

	c.mu.RLock()
	persona := c.persona
	c.mu.RUnlock()

	prompt := FormatTelemetryPrompt(persona, w)
	var rawResp string
	var err error

	if c.engine != nil {
		rawResp, err = c.engine.Generate(ctx, prompt)
	}

	if err != nil || rawResp == "" {
		if err != nil && c.logger != nil {
			c.logger.Printf("radio/llm: engine generate failed (%v), using fallback classifier", err)
		}
		if c.fallback != nil {
			return c.fallback.Classify(ctx, w)
		}
		return nil, err
	}

	jsonStr := extractJSON(rawResp)
	if jsonStr != "" {
		var resp struct {
			Category    string `json:"category"`
			Priority    int    `json:"priority"`
			Clip        string `json:"clip"`
			Text        string `json:"text"`
			CooldownSec int    `json:"cooldown_sec"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &resp); err == nil {
			if resp.Clip == "" {
				// Active silence: model explicitly determined no transmission is needed
				return nil, nil
			}
			cooldown := time.Duration(resp.CooldownSec) * time.Second
			if cooldown <= 0 {
				cooldown = 10 * time.Second
			}
			return &RadioIntent{
				ID:       resp.Clip,
				Category: IntentCategory(resp.Category),
				Priority: resp.Priority,
				ClipName: resp.Clip,
				Text:     resp.Text,
				Cooldown: cooldown,
			}, nil
		}
	}

	if c.fallback != nil {
		return c.fallback.Classify(ctx, w)
	}
	return nil, nil
}

// Close closes the underlying engine.
func (c *LLMClassifier) Close() error {
	if c.engine != nil {
		return c.engine.Close()
	}
	if c.fallback != nil {
		return c.fallback.Close()
	}
	return nil
}

func extractJSON(s string) string {
	// Strip <think>...</think> tags if present
	for {
		startTag := strings.Index(s, "<think>")
		if startTag == -1 {
			break
		}
		endTag := strings.Index(s, "</think>")
		if endTag != -1 && endTag > startTag {
			s = s[:startTag] + s[endTag+len("</think>"):]
		} else {
			s = s[:startTag]
			break
		}
	}
	s = strings.TrimSpace(s)

	// Remove markdown fences
	s = strings.ReplaceAll(s, "```json", "")
	s = strings.ReplaceAll(s, "```", "")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end >= start {
		return s[start : end+1]
	}
	return ""
}

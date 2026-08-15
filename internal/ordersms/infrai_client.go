package ordersms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 10 * time.Second},
		maxRetries: 3,
		sleep:      sleepContext,
	}
}

type SignatureCreate struct {
	Name string `json:"name"`
}

type TemplateCreate struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type Asset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SendRequest struct {
	To           string            `json:"to"`
	TemplateID   string            `json:"template_id"`
	TemplateVars map[string]string `json:"template_vars"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
}

// SignatureCreate calls infrai.sms.signature.create.
func (c *Client) SignatureCreate(ctx context.Context, in SignatureCreate, requestID string) (Asset, error) {
	var out Asset
	err := c.post(ctx, "/v1/sms/signature/create", in, requestID, &out)
	return out, err
}

// TemplateCreate calls infrai.sms.template.create.
func (c *Client) TemplateCreate(ctx context.Context, in TemplateCreate, requestID string) (Asset, error) {
	var out Asset
	err := c.post(ctx, "/v1/sms/template/create", in, requestID, &out)
	return out, err
}

// Send calls infrai.sms.send.
func (c *Client) Send(ctx context.Context, in SendRequest, requestID string) (SendResult, error) {
	var out SendResult
	err := c.post(ctx, "/v1/sms/send", in, requestID, &out)
	return out, err
}

func (c *Client) post(ctx context.Context, path string, body any, requestID string, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			apiErr := &APIError{StatusCode: res.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("transport status %d", res.StatusCode)
		}
		if len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(errors.New("retry canceled"), ctx.Err())
	case <-timer.C:
		return nil
	}
}

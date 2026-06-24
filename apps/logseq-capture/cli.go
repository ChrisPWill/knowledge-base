package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type daemonClient struct {
	baseURL    string
	httpClient *http.Client
}

func newDaemonClient(addr string) *daemonClient {
	return &daemonClient{
		baseURL:    "http://" + addr,
		httpClient: &http.Client{},
	}
}

func (c *daemonClient) status(ctx context.Context) (string, error) {
	var payload struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := c.get(ctx, "/health", &payload); err != nil {
		return "", err
	}
	return payload.Status, nil
}

func (c *daemonClient) capture(ctx context.Context, clientID, text string) (CaptureResponse, error) {
	return c.post(ctx, "/capture", textRequest{ClientID: clientID, Text: text})
}

func (c *daemonClient) command(ctx context.Context, clientID, text string) (CaptureResponse, error) {
	return c.post(ctx, "/command", textRequest{ClientID: clientID, Text: text})
}

func (c *daemonClient) review(ctx context.Context, clientID, day string) (CaptureResponse, error) {
	var resp CaptureResponse
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("day", day)
	if err := c.get(ctx, "/review?"+query.Encode(), &resp); err != nil {
		return CaptureResponse{}, err
	}
	return resp, nil
}

func (c *daemonClient) post(ctx context.Context, path string, payload any) (CaptureResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return CaptureResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return CaptureResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return CaptureResponse{}, err
	}
	defer httpResp.Body.Close()

	var resp CaptureResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return CaptureResponse{}, err
	}
	if httpResp.StatusCode >= 400 {
		if resp.Error != "" {
			return CaptureResponse{}, fmt.Errorf("%s", resp.Error)
		}
		return CaptureResponse{}, fmt.Errorf("request failed with status %d", httpResp.StatusCode)
	}
	return resp, nil
}

func (c *daemonClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode >= 400 {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(httpResp.Body).Decode(out)
}

func runInteractiveCLI(ctx context.Context, client *daemonClient, clientID string, stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	for {
		if _, err := fmt.Fprint(stdout, "> "); err != nil {
			return err
		}
		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}

		resp, err := routeCLIInput(ctx, client, clientID, line)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, resp.Reply); err != nil {
			return err
		}
	}
}

func routeCLIInput(ctx context.Context, client *daemonClient, clientID, line string) (CaptureResponse, error) {
	lower := strings.ToLower(strings.TrimSpace(line))
	switch {
	case lower == "/today":
		return client.review(ctx, clientID, "today")
	case lower == "/yesterday":
		return client.review(ctx, clientID, "yesterday")
	case lower == "help" || strings.HasPrefix(lower, "help ") || lower == "toggle also":
		return client.command(ctx, clientID, line)
	default:
		return client.capture(ctx, clientID, line)
	}
}

func daemonAddr() string {
	if value := os.Getenv("LOGSEQ_CAPTURE_ADDR"); value != "" {
		return value
	}
	return defaultDaemonAddr
}

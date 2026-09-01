package iqclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultEndpoint       = "https://binary-options.mcp.iqoption.com"
	DefaultProtocol       = "2025-06-18"
	DefaultClientName     = "iqoption-mcp-client"
	DefaultClientVersion  = "1.0.0"
	DefaultHTTPTimeout    = 60 * time.Second
	DefaultMaxRetries     = 3
	DefaultRetryBaseDelay = 500 * time.Millisecond
)

// Config configures an IQ Option MCP client.
type Config struct {
	Endpoint string
	Token    string

	ProtocolVersion string

	ClientName    string
	ClientVersion string

	HTTPClient *http.Client

	MaxRetries     int
	RetryBaseDelay time.Duration

	// Optional hooks for observability.
	OnRequest  func(method string)
	OnResponse func(method string, duration time.Duration, err error)
}

// Client is a resilient MCP client for the IQ Option MCP server.
//
// Client is safe for concurrent use.
type Client struct {
	endpoint string
	token    string
	protocol string

	clientName    string
	clientVersion string

	httpClient *http.Client

	maxRetries     int
	retryBaseDelay time.Duration

	sessionMu sync.RWMutex
	sessionID string

	requestID atomic.Uint64

	closeOnce sync.Once
	closed    atomic.Bool
}

// New creates a new IQ Option MCP client.
//
// No network request is made by New. The MCP session is initialized lazily
// on the first request.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("iqclient: token is required")
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	protocol := strings.TrimSpace(cfg.ProtocolVersion)
	if protocol == "" {
		protocol = DefaultProtocol
	}

	clientName := cfg.ClientName
	if clientName == "" {
		clientName = DefaultClientName
	}

	clientVersion := cfg.ClientVersion
	if clientVersion == "" {
		clientVersion = DefaultClientVersion
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: DefaultHTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}

	retryDelay := cfg.RetryBaseDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryBaseDelay
	}

	return &Client{
		endpoint:       endpoint,
		token:          cfg.Token,
		protocol:       protocol,
		clientName:     clientName,
		clientVersion:  clientVersion,
		httpClient:     httpClient,
		maxRetries:     maxRetries,
		retryBaseDelay: retryDelay,
	}, nil
}

// Close releases the client.
//
// The HTTP client itself is not closed because ownership belongs to Config
// when a custom HTTP client is supplied.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
	})

	return nil
}

// IsClosed reports whether Close has been called.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}

func (c *Client) nextRequestID() uint64 {
	return c.requestID.Add(1)
}

func (c *Client) getSessionID() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()

	return c.sessionID
}

func (c *Client) setSessionID(id string) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	c.sessionID = id
}

func (c *Client) clearSession() {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	c.sessionID = ""
}

// initialize establishes the MCP session.
//
// The IQ Option server currently advertises MCP protocol 2025-06-18.
func (c *Client) initialize(ctx context.Context) error {
	if c.IsClosed() {
		return ErrClosed
	}

	if c.getSessionID() != "" {
		return nil
	}

	// Only serialize the initialization decision.
	// Do NOT hold sessionMu while performing network I/O.
	c.sessionMu.Lock()

	if c.sessionID != "" {
		c.sessionMu.Unlock()
		return nil
	}

	c.sessionMu.Unlock()

	params := InitializeParams{
		ProtocolVersion: c.protocol,
		Capabilities:    map[string]any{},
		ClientInfo: ClientInfo{
			Name:    c.clientName,
			Version: c.clientVersion,
		},
	}

	result, sessionID, err := c.post(ctx, "initialize", params, false)
	if err != nil {
		return fmt.Errorf("iqclient: initialize: %w", err)
	}

	if sessionID == "" {
		return errors.New(
			"iqclient: initialize response did not contain mcp-session-id",
		)
	}

	var response InitializeResult

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf(
			"iqclient: decode initialize response: %w",
			err,
		)
	}

	if response.ProtocolVersion == "" {
		return errors.New(
			"iqclient: server returned empty protocol version",
		)
	}

	// Store the session only after successful initialization.
	c.sessionMu.Lock()
	c.sessionID = sessionID
	c.sessionMu.Unlock()

	// MCP requires the initialized notification after initialization.
	if _, _, err := c.post(
		ctx,
		"notifications/initialized",
		map[string]any{},
		true,
	); err != nil {
		c.clearSession()

		return fmt.Errorf(
			"iqclient: initialized notification: %w",
			err,
		)
	}

	return nil
}

func (c *Client) call(
	ctx context.Context,
	method string,
	params any,
) ([]byte, error) {
	if c.IsClosed() {
		return nil, ErrClosed
	}

	if err := c.initialize(ctx); err != nil {
		return nil, err
	}

	result, _, err := c.post(ctx, method, params, false)
	if err == nil {
		return result, nil
	}

	// A stale session should be recoverable.
	if errors.Is(err, ErrSessionExpired) {
		c.clearSession()

		if initErr := c.initialize(ctx); initErr != nil {
			return nil, fmt.Errorf(
				"iqclient: session recovery failed: %w",
				initErr,
			)
		}

		result, _, retryErr := c.post(ctx, method, params, false)
		if retryErr != nil {
			return nil, retryErr
		}

		return result, nil
	}

	return nil, err
}

func (c *Client) callTool(
	ctx context.Context,
	tool string,
	arguments map[string]any,
) ([]byte, error) {
	result, err := c.call(
		ctx,
		"tools/call",
		ToolCallParams{
			Name:      tool,
			Arguments: arguments,
		},
	)
	if err != nil {
		return nil, err
	}

	return decodeMCPToolResult(result)
}

func decodeMCPToolResult(result []byte) ([]byte, error) {
	var response MCPToolResult

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf(
			"decode MCP tool response: %w",
			err,
		)
	}

	if response.IsError {
		for _, content := range response.Content {
			if content.Type != "text" {
				continue
			}

			message := strings.TrimSpace(content.Text)
			if message == "" {
				continue
			}

			return nil, &MCPError{
				Message: message,
			}
		}

		return nil, &MCPError{
			Message: "MCP tool returned an error",
		}
	}

	// Current IQ Option MCP server provides structuredContent.
	if len(response.StructuredContent) > 0 &&
		string(response.StructuredContent) != "null" {
		return response.StructuredContent, nil
	}

	// Fallback for MCP servers that only provide text content.
	for _, content := range response.Content {
		if content.Type != "text" {
			continue
		}

		text := strings.TrimSpace(content.Text)

		if text != "" && json.Valid([]byte(text)) {
			return []byte(text), nil
		}
	}

	return nil, errors.New(
		"no JSON content found in MCP tool response",
	)
}

func decodeMCPToolError(
	result MCPToolResult,
) error {
	for _, content := range result.Content {
		if content.Type == "text" {
			message := strings.TrimSpace(content.Text)

			if message != "" {
				return &MCPError{
					Message: message,
				}
			}
		}
	}

	return &MCPError{
		Message: "MCP tool returned an error",
	}
}

// post sends one MCP JSON-RPC request.
//
// IMPORTANT:
// retryable controls whether the request itself can safely be retried.
//
// We deliberately do NOT retry mutations such as place_trade. A network
// timeout after a successful trade must never result in a second trade.
func (c *Client) post(
	ctx context.Context,
	method string,
	params any,
	notification bool,
) ([]byte, string, error) {
	requestID := c.nextRequestID()

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	if !notification {
		request.ID = requestID
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encode request: %w", err)
	}

	var lastErr error

	attempts := 1
	if !notification && isRetryableMCPMethod(method) {
		attempts += c.maxRetries
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := c.retryBaseDelay * time.Duration(1<<(attempt-1))

			if err := sleepContext(ctx, delay); err != nil {
				return nil, "", err
			}
		}

		start := time.Now()

		if err := c.notifyRequest(method); err != nil {
			return nil, "", err
		}

		result, sessionID, err := c.doHTTP(ctx, body)

		if c.notifyResponse(method, time.Since(start), err); err != nil {
			return nil, "", err
		}

		if err == nil {
			return result, sessionID, nil
		}

		lastErr = err

		if !isRetryableError(err) {
			break
		}
	}

	return nil, "", fmt.Errorf(
		"mcp request %q failed after retries: %w",
		method,
		lastErr,
	)
}

func (c *Client) doHTTP(
	ctx context.Context,
	body []byte,
) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, "", fmt.Errorf("create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(
		"Accept",
		"application/json, text/event-stream",
	)

	if sessionID := c.getSessionID(); sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	req.Header.Set(
		"MCP-Protocol-Version",
		c.protocol,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", classifyNetworkError(err)
	}
	defer resp.Body.Close()

	sessionID := resp.Header.Get("Mcp-Session-Id")

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, sessionID, fmt.Errorf("read HTTP response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden {
		return nil, sessionID, fmt.Errorf(
			"%w: HTTP %d",
			ErrUnauthorized,
			resp.StatusCode,
		)
	}

	if resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusGone {
		return nil, sessionID, ErrSessionExpired
	}

	if resp.StatusCode >= 500 {
		return nil, sessionID, fmt.Errorf(
			"%w: HTTP %d: %s",
			ErrServerUnavailable,
			resp.StatusCode,
			summarizeBody(responseBody),
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, sessionID, fmt.Errorf(
			"HTTP %d: %s",
			resp.StatusCode,
			summarizeBody(responseBody),
		)
	}

	if len(bytes.TrimSpace(responseBody)) == 0 {
		return nil, sessionID, nil
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	result, err := decodeMCPResponse(responseBody, contentType)
	if err != nil {
		return nil, sessionID, err
	}

	var rpc JSONRPCResponse
	if err := json.Unmarshal(result, &rpc); err != nil {
		return nil, sessionID, fmt.Errorf(
			"decode JSON-RPC response: %w",
			err,
		)
	}

	if rpc.Error != nil {
		return nil, sessionID, &MCPError{
			Code:    rpc.Error.Code,
			Message: rpc.Error.Message,
			Data:    rpc.Error.Data,
		}
	}

	return rpc.Result, sessionID, nil
}

func decodeMCPResponse(
	body []byte,
	contentType string,
) ([]byte, error) {
	body = bytes.TrimSpace(body)

	if len(body) == 0 {
		return nil, errors.New("empty MCP response")
	}

	if json.Valid(body) {
		return body, nil
	}

	if strings.Contains(
		strings.ToLower(contentType),
		"text/event-stream",
	) {
		return decodeMCPSSE(body)
	}

	return nil, fmt.Errorf(
		"invalid MCP response: content-type=%q body=%q",
		contentType,
		string(body),
	)
}

func decodeMCPJSON(body []byte) ([]byte, error) {
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`

		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf(
			"decode MCP JSON response: %w",
			err,
		)
	}

	if len(envelope.Error) > 0 &&
		string(envelope.Error) != "null" {
		var mcpErr MCPError

		if err := json.Unmarshal(envelope.Error, &mcpErr); err != nil {
			return nil, fmt.Errorf(
				"decode MCP error: %w",
				err,
			)
		}

		return nil, &mcpErr
	}

	if len(envelope.Result) == 0 ||
		string(envelope.Result) == "null" {
		return nil, errors.New(
			"MCP response contains no result",
		)
	}

	// First, return the result directly if it isn't a
	// normal tools/call result envelope.
	//
	// tools/call normally contains:
	//
	// {
	//   "content": [
	//     {
	//       "type": "text",
	//       "text": "[...]"
	//     }
	//   ]
	// }
	var result struct {
		Content           []MCPContent    `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}

	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, fmt.Errorf(
			"decode MCP result: %w",
			err,
		)
	}

	if len(result.StructuredContent) > 0 &&
		string(result.StructuredContent) != "null" {
		return result.StructuredContent, nil
	}

	for _, content := range result.Content {
		if content.Type != "text" {
			continue
		}

		text := strings.TrimSpace(content.Text)

		if text == "" {
			continue
		}

		// IQ Option's MCP server puts the actual tool JSON
		// inside the text field.
		if json.Valid([]byte(text)) {
			return []byte(text), nil
		}
	}

	// If this wasn't a tools/call-style response, return
	// the raw result so callers can decode it themselves.
	return envelope.Result, nil
}

func decodeMCPSSE(body []byte) ([]byte, error) {
	lines := strings.Split(
		strings.ReplaceAll(
			string(body),
			"\r\n",
			"\n",
		),
		"\n",
	)

	var dataLines []string

	flush := func() ([]byte, bool) {
		if len(dataLines) == 0 {
			return nil, false
		}

		data := strings.TrimSpace(
			strings.Join(dataLines, "\n"),
		)

		dataLines = nil

		if data == "" || data == "[DONE]" {
			return nil, false
		}

		if !json.Valid([]byte(data)) {
			return nil, false
		}

		return []byte(data), true
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			dataLines = append(
				dataLines,
				strings.TrimSpace(
					strings.TrimPrefix(line, "data:"),
				),
			)
			continue
		}

		if line == "" {
			if result, ok := flush(); ok {
				return result, nil
			}
		}
	}

	if result, ok := flush(); ok {
		return result, nil
	}

	return nil, errors.New(
		"no JSON content found in MCP SSE response",
	)
}

func isRetryableMCPMethod(method string) bool {
	switch method {
	case "initialize",
		"tools/list",
		"tools/call":
		return true
	default:
		return false
	}
}

func isRetryableError(err error) bool {
	return errors.Is(err, ErrNetwork) ||
		errors.Is(err, ErrServerUnavailable)
}

func classifyNetworkError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %v", ErrNetwork, err)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func summarizeBody(body []byte) string {
	const max = 500

	text := strings.TrimSpace(string(body))

	if len(text) > max {
		return text[:max] + "..."
	}

	return text
}

func (c *Client) notifyRequest(method string) error {
	return nil
}

func (c *Client) notifyResponse(
	method string,
	duration time.Duration,
	err error,
) error {
	return nil
}

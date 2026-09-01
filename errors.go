package iqclient

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrClosed            = errors.New("iqclient: client is closed")
	ErrNetwork           = errors.New("iqclient: network error")
	ErrUnauthorized      = errors.New("iqclient: unauthorized")
	ErrServerUnavailable = errors.New("iqclient: server unavailable")
	ErrSessionExpired    = errors.New("iqclient: MCP session expired")
	ErrValidation        = errors.New("iqclient: validation error")
	ErrTradingDenied     = errors.New("iqclient: trading access denied")
	ErrToolNotFound      = errors.New("iqclient: MCP tool not found")
)

// MCPError represents an MCP/JSON-RPC error returned by the server.
type MCPError struct {
	Code    int
	Message string
	Data    any
}

func (e *MCPError) Error() string {
	if e.Data == nil {
		return fmt.Sprintf(
			"mcp error %d: %s",
			e.Code,
			e.Message,
		)
	}

	return fmt.Sprintf(
		"mcp error %d: %s (%v)",
		e.Code,
		e.Message,
		e.Data,
	)
}

func (e *MCPError) Is(target error) bool {
	if target == nil {
		return false
	}

	message := strings.ToLower(e.Message)

	switch {
	case errors.Is(target, ErrTradingDenied):
		return strings.Contains(
			message,
			"trading_access_denied",
		)

	case errors.Is(target, ErrValidation):
		return strings.Contains(
			message,
			"validation",
		)

	case errors.Is(target, ErrToolNotFound):
		return strings.Contains(
			message,
			"tool not found",
		)
	}

	return false
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if s == value {
			return true
		}
	}

	return false
}

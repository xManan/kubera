package mcp

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/domain"
)

// errorPayload is the stable error envelope from the LLD section 10.1.
type errorPayload struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// toolOK builds a successful structured result with a text fallback.
func toolOK(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return internalResult(), nil, nil
	}
	return &mcp.CallToolResult{
		StructuredContent: v,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// toolErr maps an application error to a safe structured MCP error result.
// Internal and database errors are logged with detail but return a generic
// message; no SQL, paths, or stack traces reach the client.
func toolErr(err error, log *slog.Logger) (*mcp.CallToolResult, any, error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		log.Error("internal tool error", "error", err)
		return internalResult(), nil, nil
	}
	switch de.Code {
	case domain.CodeInternal, domain.CodeDatabaseUnavailable, domain.CodeMigrationFailed:
		log.Error("tool error", "code", de.Code, "detail", de.Message, "cause", err.Error())
	default:
		log.Info("tool error", "code", de.Code)
	}
	return errorResult(errorBody{
		Code:    string(de.Code),
		Message: de.SafeMessage(),
		Details: de.Details,
	}), nil, nil
}

func internalResult() *mcp.CallToolResult {
	return errorResult(errorBody{
		Code:    string(domain.CodeInternal),
		Message: "An internal error occurred. The operation could not be completed.",
	})
}

func errorResult(body errorBody) *mcp.CallToolResult {
	p := errorPayload{Error: body}
	b, _ := json.Marshal(p)
	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: p,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}
}

package mailbox

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ToolResult(value any) (*mcp.CallToolResult, error) {
	encoded, err := Encode(value)
	if err != nil {
		return nil, ErrUnavailable
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}
	wire, err := json.Marshal(result)
	if err != nil || len(wire) > MaxToolResultBytes-ToolResultMargin {
		return nil, ErrUnavailable
	}
	return result, nil
}

func fits(value any) bool { _, err := ToolResult(value); return err == nil }

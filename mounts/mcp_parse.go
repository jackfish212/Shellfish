package mounts

import "encoding/json"

func parseToolsList(data []byte) ([]MCPTool, error) {
	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description,omitempty"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	tools := make([]MCPTool, len(result.Tools))
	for i, t := range result.Tools {
		tools[i] = MCPTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
	}
	return tools, nil
}

func parseToolCallResult(data []byte) (*MCPToolResult, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	content := make([]MCPContent, len(result.Content))
	for i, c := range result.Content {
		content[i] = MCPContent{Type: c.Type, Text: c.Text}
	}
	return &MCPToolResult{Content: content, IsError: result.IsError}, nil
}

func parseResourcesList(data []byte) ([]MCPResource, error) {
	var result struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			MimeType    string `json:"mimeType,omitempty"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, nil
	}
	resources := make([]MCPResource, len(result.Resources))
	for i, r := range result.Resources {
		resources[i] = MCPResource{URI: r.URI, Name: r.Name, Description: r.Description, MimeType: r.MimeType}
	}
	return resources, nil
}

func parseResourceRead(data []byte) (string, error) {
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			Text     string `json:"text,omitempty"`
			MimeType string `json:"mimeType,omitempty"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", nil
	}
	if len(result.Contents) > 0 {
		return result.Contents[0].Text, nil
	}
	return "", nil
}

func parsePromptsList(data []byte) ([]MCPPrompt, error) {
	var result struct {
		Prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, nil
	}
	prompts := make([]MCPPrompt, len(result.Prompts))
	for i, p := range result.Prompts {
		prompts[i] = MCPPrompt{Name: p.Name, Description: p.Description}
	}
	return prompts, nil
}

func parsePromptGet(data []byte) (string, error) {
	var result struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", nil
	}
	var text string
	for _, m := range result.Messages {
		if m.Content.Type == "text" {
			text += m.Content.Text + "\n"
		}
	}
	return text, nil
}

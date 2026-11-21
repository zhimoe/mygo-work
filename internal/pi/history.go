package pi

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"mygo-work/internal/agent"
)

type wireMessage struct {
	Role         string         `json:"role"`
	Timestamp    int64          `json:"timestamp"`
	Content      jsontext.Value `json:"content"`
	ToolCallID   string         `json:"toolCallId"`
	ErrorMessage string         `json:"errorMessage"`
	StopReason   string         `json:"stopReason"`
	IsError      bool           `json:"isError"`
	Partial      []wirePart     `json:"-"`
}

type wirePart struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Thinking  string `json:"thinking"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

func (m wireMessage) parts() []wirePart {
	if m.Partial != nil {
		return m.Partial
	}
	content := bytes.TrimSpace(m.Content)
	if len(content) == 0 {
		return nil
	}
	if content[0] == '"' {
		var text string
		if json.Unmarshal(content, &text) == nil {
			return []wirePart{{Type: "text", Text: text}}
		}
		return nil
	}
	var parts []wirePart
	_ = json.Unmarshal(content, &parts)
	return parts
}

func (m *wireMessage) apply(update assistantUpdate) {
	kind, _, _ := strings.Cut(update.Type, "_")
	index := update.Index
	if index < 0 || index > 10000 || (kind != "text" && kind != "thinking" && kind != "toolcall") {
		return
	}
	if index >= len(m.Partial) {
		m.Partial = append(m.Partial, make([]wirePart, index-len(m.Partial)+1)...)
	}
	part := &m.Partial[index]
	switch update.Type {
	case "text_start":
		part.Type = "text"
	case "text_delta":
		part.Type = "text"
		part.Text += update.Delta
	case "text_end":
		part.Type = "text"
		part.Text = update.Content
	case "thinking_start":
		part.Type = "thinking"
	case "thinking_delta":
		part.Type = "thinking"
		part.Thinking += update.Delta
	case "thinking_end":
		part.Type = "thinking"
		part.Thinking = update.Content
	case "toolcall_start":
		*part = wirePart{Type: "toolCall", ID: update.ID, Name: update.ToolName}
	case "toolcall_end":
		*part = update.ToolCall
		part.Type = "toolCall"
	}
}

func (m wireMessage) toolOutput() string {
	var lines []string
	for _, part := range m.parts() {
		if part.Type == "text" {
			lines = append(lines, part.Text)
		}
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) History(ctx context.Context, id, cursor string) (agent.History, error) {
	_, conn, err := r.get(id)
	if err != nil {
		return agent.History{}, err
	}
	var response struct {
		Messages []wireMessage `json:"messages"`
	}
	if err := conn.call(ctx, "get_messages", nil, &response); err != nil {
		return agent.History{}, err
	}
	conn.mu.Lock()
	var live *wireMessage
	if conn.preview != nil {
		preview := *conn.preview
		preview.Partial = slices.Clone(preview.Partial)
		live = &preview
	}
	conn.mu.Unlock()
	messages := convert(response.Messages, live, conn.busy.Load())
	end := len(messages)
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 {
			return agent.History{}, errors.New("invalid Pi history cursor")
		}
		end = min(end, value)
	}
	start := max(0, end-50)
	previous := ""
	if start > 0 {
		previous = strconv.Itoa(start)
	}
	return agent.History{Messages: messages[start:end], Previous: previous}, nil
}

func convert(records []wireMessage, live *wireMessage, active bool) []agent.Message {
	results := make(map[string]wireMessage)
	for _, item := range records {
		if item.Role == "toolResult" {
			results[item.ToolCallID] = item
		}
	}
	last := len(records) - 1
	if live != nil && (last < 0 || records[last].Role != "assistant" || records[last].Timestamp != live.Timestamp) {
		records = append(records, *live)
		live = nil
	}
	ids := make(map[string]int)
	out := make([]agent.Message, 0, len(records))
	for i, item := range records {
		if item.Role != "user" && item.Role != "assistant" {
			continue
		}
		if i == last && live != nil {
			item = *live
		}
		order := 0
		if item.Role == "assistant" {
			order = 1
		}
		prefix := fmt.Sprintf("%016d-%d", item.Timestamp, order)
		id := fmt.Sprintf("%s-%06d", prefix, ids[prefix])
		ids[prefix]++
		message := agent.Message{ID: id, Role: item.Role, Error: item.ErrorMessage,
			Complete: item.Role == "user" || item.StopReason != ""}
		for _, part := range item.parts() {
			switch part.Type {
			case "text":
				message.Parts = append(message.Parts, agent.Part{Kind: "text", Text: part.Text})
			case "thinking":
				message.Parts = append(message.Parts, agent.Part{Kind: "reasoning", Text: part.Thinking})
			case "toolCall":
				value := agent.Part{Kind: "tool", Name: part.Name, Input: part.Arguments, Status: "running"}
				if result, ok := results[part.ID]; ok {
					value.Status = "completed"
					if result.IsError {
						value.Status = "error"
					}
					value.Output = result.toolOutput()
				} else if !active {
					value.Status = "stopped"
				}
				message.Parts = append(message.Parts, value)
			}
		}
		out = append(out, message)
	}
	return out
}

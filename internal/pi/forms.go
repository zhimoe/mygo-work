package pi

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"slices"
	"time"

	"mygo-work/internal/agent"
)

type pendingForm struct {
	form    agent.Form
	method  string
	expires time.Time
}

func (event record) form() (pendingForm, bool) {
	if event.ID == "" {
		return pendingForm{}, false
	}
	field := agent.Field{Key: "response", Title: "Response", Type: "string", Required: true}
	switch event.Method {
	case "select":
		for _, option := range event.Options {
			field.Options = append(field.Options, agent.Choice{Label: option, Value: option})
		}
	case "confirm":
		field.Type = "boolean"
		_ = json.Unmarshal(event.Message, &field.Title)
	case "input":
		field.Description = event.Placeholder
	case "editor":
		field.Type = "editor"
		field.Default = event.Prefill
	default:
		return pendingForm{}, false
	}
	form := pendingForm{method: event.Method, form: agent.Form{ID: event.ID, Title: event.Title, Fields: []agent.Field{field}}}
	if event.Timeout > 0 {
		form.expires = time.Now().Add(time.Duration(min(event.Timeout, 86_400_000)) * time.Millisecond)
	}
	return form, true
}

func (conn *connection) formsSnapshot() []agent.Form {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	var forms []agent.Form
	for id, entry := range conn.forms {
		if !entry.expires.IsZero() && time.Now().After(entry.expires) {
			delete(conn.forms, id)
		} else {
			forms = append(forms, entry.form)
		}
	}
	slices.SortFunc(forms, func(a, b agent.Form) int { return cmp.Compare(a.ID, b.ID) })
	return forms
}

func (conn *connection) respond(ctx context.Context, id string, values map[string]any) error {
	conn.mu.Lock()
	entry, ok := conn.forms[id]
	conn.mu.Unlock()
	if !ok || !entry.expires.IsZero() && time.Now().After(entry.expires) {
		return errors.New("Pi interaction is no longer pending")
	}
	reply := map[string]any{"type": "extension_ui_response", "id": id}
	switch {
	case values == nil:
		reply["cancelled"] = true
	case entry.method == "confirm":
		confirmed, ok := values["response"].(bool)
		if !ok {
			return errors.New("choose Yes or No")
		}
		reply["confirmed"] = confirmed
	default:
		value, ok := values["response"].(string)
		if !ok {
			return errors.New("enter a response")
		}
		if entry.method == "select" && !slices.ContainsFunc(entry.form.Fields[0].Options, func(option agent.Choice) bool { return option.Value == value }) {
			return errors.New("choose an available response")
		}
		reply["value"] = value
	}
	if err := conn.write(ctx, reply); err != nil {
		return err
	}
	conn.mu.Lock()
	delete(conn.forms, id)
	conn.mu.Unlock()
	return nil
}

func (r *Runtime) Interactions(_ context.Context, id string) (agent.Interactions, error) {
	r.mu.Lock()
	conn := r.connections[id]
	r.mu.Unlock()
	if conn == nil {
		return agent.Interactions{}, nil
	}
	return agent.Interactions{Forms: conn.formsSnapshot()}, nil
}

func (r *Runtime) Decide(context.Context, string, string, string) error {
	return errors.New("Pi tool approval is managed by Pi")
}

func (r *Runtime) Answer(ctx context.Context, id, form string, answer map[string]any) error {
	r.mu.Lock()
	conn := r.connections[id]
	entry := r.sessions[id]
	r.mu.Unlock()
	if conn == nil {
		return errors.New("Pi interaction is no longer pending")
	}
	if err := conn.respond(ctx, form, answer); err != nil {
		return err
	}
	r.emit(id, entry.Session.Directory, "change")
	return nil
}

package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"mygo-work/internal/agent"
)

var _ agent.Runtime = (*Process)(nil)

type response[T any] struct {
	Data T `json:"data"`
}

type modelRef struct {
	Provider string `json:"providerID"`
	ID       string `json:"id"`
	Variant  string `json:"variant,omitempty"`
}

func (r modelRef) ref() agent.ModelRef {
	return agent.ModelRef{Provider: r.Provider, ID: r.ID, Variant: r.Variant}
}

type session struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Mode     string   `json:"agent"`
	Model    modelRef `json:"model"`
	Location struct {
		Directory string `json:"directory"`
	} `json:"location"`
	Time struct {
		Updated  int64 `json:"updated"`
		Archived int64 `json:"archived"`
	} `json:"time"`
}

func (s session) value() agent.Session {
	return agent.Session{ID: s.ID, Title: s.Title, Mode: s.Mode, Model: s.Model.ref(),
		Directory: s.Location.Directory, Updated: s.Time.Updated, Archived: s.Time.Archived > 0}
}

func readError(method, path string, res *http.Response) error {
	var payload struct {
		Type    string `json:"_tag"`
		Message string `json:"message"`
	}
	if method == "GET" {
		_ = json.UnmarshalRead(io.LimitReader(res.Body, 32<<10), &payload)
	}
	detail := strings.TrimSpace(payload.Message)
	if detail == "" {
		detail = payload.Type
	}
	if detail != "" {
		return fmt.Errorf("OpenCode %s %s: HTTP %d · %.300s", method, strings.Split(path, "?")[0], res.StatusCode, detail)
	}
	return fmt.Errorf("OpenCode %s %s: HTTP %d", method, strings.Split(path, "?")[0], res.StatusCode)
}

func (c *Client) call(ctx context.Context, method, path string, data, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var body io.Reader
	if data != nil {
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	res, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if out == nil {
		_, err := io.Copy(io.Discard, res.Body)
		return err
	}
	return json.UnmarshalRead(io.LimitReader(res.Body, 24<<20), out)
}

func (p *Process) call(ctx context.Context, method, path string, data, out any) error {
	client, err := p.client(ctx)
	if err != nil {
		return err
	}
	return client.call(ctx, method, path, data, out)
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var info struct {
		Version string `json:"version"`
	}
	if err := c.call(ctx, "GET", "/api/info", nil, &info); err != nil {
		return "", err
	}
	if !v2Version.MatchString(info.Version) {
		return "", fmt.Errorf("incompatible OpenCode server version %q", info.Version)
	}
	return info.Version, nil
}

func (p *Process) Version(ctx context.Context) (string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return "", err
	}
	return client.Version(ctx)
}

func sessionPath(id, suffix string) string {
	return "/api/session/" + url.PathEscape(id) + suffix
}

func scoped(path, directory string) string {
	return path + "?" + url.Values{"location[directory]": {directory}}.Encode()
}

func (p *Process) Sessions(ctx context.Context, dir string, archived bool, search string) ([]agent.Session, error) {
	query := url.Values{"directory": {dir}, "order": {"desc"}, "parentID": {"null"}, "limit": {"100"}, "archived": {fmt.Sprint(archived)}}
	if search != "" {
		query.Set("search", search)
	}
	var res response[[]session]
	if err := p.call(ctx, "GET", "/api/session?"+query.Encode(), nil, &res); err != nil {
		return nil, err
	}
	out := make([]agent.Session, 0, len(res.Data))
	for _, item := range res.Data {
		out = append(out, item.value())
	}
	return out, nil
}

func taskPermissions(mode string) []map[string]string {
	if mode != "" && mode != "build" {
		return []map[string]string{}
	}
	return []map[string]string{
		{"action": "edit", "resource": "*", "effect": "ask"},
		{"action": "shell", "resource": "*", "effect": "ask"},
		{"action": "external_directory", "resource": "*", "effect": "ask"},
	}
}

func (p *Process) Create(ctx context.Context, dir string, prompt agent.Prompt) (agent.Session, error) {
	if err := p.prepareConnections(ctx, dir); err != nil {
		return agent.Session{}, err
	}
	body := map[string]any{
		"location":    map[string]string{"directory": dir},
		"permissions": taskPermissions(prompt.Mode),
	}
	if prompt.Model.ID != "" {
		body["model"] = modelRef(prompt.Model)
	}
	if prompt.Mode != "" {
		body["agent"] = prompt.Mode
	}
	var res response[session]
	err := p.call(ctx, "POST", "/api/session", body, &res)
	return res.Data.value(), err
}

func (p *Process) Update(ctx context.Context, id string, patch agent.SessionPatch) error {
	return p.call(ctx, "PATCH", sessionPath(id, ""), patch, nil)
}

func (p *Process) Delete(ctx context.Context, id string) error {
	return p.call(ctx, "DELETE", sessionPath(id, ""), nil, nil)
}

func (p *Process) Options(ctx context.Context, dir string) (agent.Options, error) {
	if err := p.prepareConnections(ctx, dir); err != nil {
		return agent.Options{}, err
	}
	var models response[[]struct {
		modelRef
		Name     string `json:"name"`
		Type     string `json:"type"`
		Enabled  bool   `json:"enabled"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}]
	var modes response[[]struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Mode   string `json:"mode"`
		Hidden bool   `json:"hidden"`
	}]
	var defaultModel response[modelRef]
	if err := p.call(ctx, "GET", scoped("/api/location", dir), nil, nil); err != nil {
		return agent.Options{}, err
	}
	var modelErr, modeErr error
	var wg sync.WaitGroup
	wg.Go(func() { modelErr = p.call(ctx, "GET", scoped("/api/model", dir), nil, &models) })
	wg.Go(func() { modeErr = p.call(ctx, "GET", scoped("/api/agent", dir), nil, &modes) })
	wg.Go(func() { _ = p.call(ctx, "GET", scoped("/api/model/default", dir), nil, &defaultModel) })
	wg.Wait()
	if err := errors.Join(modelErr, modeErr); err != nil {
		return agent.Options{}, err
	}
	if len(modes.Data) == 0 {
		if err := p.call(ctx, "GET", scoped("/api/agent", dir), nil, &modes); err != nil {
			return agent.Options{}, err
		}
	}
	out := agent.Options{DefaultModel: defaultModel.Data.ref()}
	for _, item := range models.Data {
		if (item.Type != "" && item.Type != "chat") || !item.Enabled {
			continue
		}
		model := agent.Model{ModelRef: item.ref(), Name: item.Name}
		for _, variant := range item.Variants {
			model.Variants = append(model.Variants, variant.ID)
		}
		out.Models = append(out.Models, model)
	}
	for _, item := range modes.Data {
		if item.Mode != "subagent" && !item.Hidden {
			out.Modes = append(out.Modes, agent.Mode{ID: item.ID, Name: item.Name})
		}
	}
	return out, nil
}

func (p *Process) Prompt(ctx context.Context, id string, prompt agent.Prompt) error {
	if prompt.Model.ID != "" || prompt.Mode != "" {
		var current response[session]
		path := sessionPath(id, "")
		if err := p.call(ctx, "GET", path, nil, &current); err != nil {
			return err
		}
		if requested := modelRef(prompt.Model); requested.ID != "" && current.Data.Model != requested {
			if err := p.call(ctx, "POST", path+"/model", map[string]modelRef{"model": requested}, nil); err != nil {
				return err
			}
		}
		if prompt.Mode != "" && current.Data.Mode != prompt.Mode {
			if err := p.call(ctx, "POST", path+"/agent", map[string]string{"agent": prompt.Mode}, nil); err != nil {
				return err
			}
			if err := p.call(ctx, "PATCH", path, map[string]any{"permissions": taskPermissions(prompt.Mode)}, nil); err != nil {
				return err
			}
		}
	}
	body := map[string]any{"text": prompt.Text}
	if len(prompt.Files) > 0 {
		files := make([]map[string]string, 0, len(prompt.Files))
		for _, file := range prompt.Files {
			files = append(files, map[string]string{"uri": file.URI, "name": file.Name})
		}
		body["files"] = files
	}
	if len(prompt.Skills) > 0 {
		skills := make([]map[string]string, 0, len(prompt.Skills))
		for _, skill := range prompt.Skills {
			skills = append(skills, map[string]string{"id": skill})
		}
		body["skills"] = skills
	}
	return p.call(ctx, "POST", sessionPath(id, "/prompt"), body, nil)
}

func (p *Process) Interrupt(ctx context.Context, id string) error {
	return p.call(ctx, "POST", sessionPath(id, "/interrupt"), nil, nil)
}

func (p *Process) Active(ctx context.Context) ([]string, error) {
	var res response[map[string]struct{}]
	err := p.call(ctx, "GET", "/api/session/active", nil, &res)
	return slices.Collect(maps.Keys(res.Data)), err
}

type message struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Text    string   `json:"text"`
	Outcome string   `json:"outcome"`
	Error   apiError `json:"error"`
	Time    struct {
		Completed int64 `json:"completed"`
	} `json:"time"`

	Files []struct {
		Name string `json:"name"`
	} `json:"files"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Name  string `json:"name"`
		State struct {
			Status  string         `json:"status"`
			Input   any            `json:"input"`
			Content jsontext.Value `json:"content"`
			Error   apiError       `json:"error"`
			Time    struct {
				Completed int64 `json:"completed"`
			} `json:"time"`
		} `json:"state"`
	} `json:"content"`
}

type apiError struct {
	Message string `json:"message"`
}

func (m message) value() agent.Message {
	out := agent.Message{ID: m.ID, Role: m.Type, Outcome: m.Outcome, Error: m.Error.Message, Complete: m.Time.Completed > 0}
	if m.Text != "" {
		out.Parts = append(out.Parts, agent.Part{Kind: "text", Text: m.Text})
	}
	for _, item := range m.Content {
		part := agent.Part{Kind: item.Type, Text: item.Text, Name: item.Name, Status: item.State.Status, Input: item.State.Input}
		if part.Kind == "tool" {
			part.Output = item.State.Error.Message
			var entry struct {
				Text string `json:"text"`
				URI  string `json:"uri"`
			}
			if len(item.State.Content) > 0 && json.Unmarshal(item.State.Content, &entry) == nil {
				part.Output = strings.TrimSpace(part.Output + "\n" + entry.Text + entry.URI)
			}
		}
		out.Parts = append(out.Parts, part)
	}
	for _, file := range m.Files {
		out.Files = append(out.Files, agent.Attachment{Name: file.Name})
	}
	return out
}

func (p *Process) History(ctx context.Context, id, cursor string) (agent.History, error) {
	query := url.Values{"limit": {"50"}}
	if cursor == "" {
		query.Set("order", "desc")
	} else {
		query.Set("cursor", cursor)
	}
	var res struct {
		Data   []message `json:"data"`
		Cursor struct {
			Next string `json:"next"`
		} `json:"cursor"`
	}
	err := p.call(ctx, "GET", sessionPath(id, "/message?")+query.Encode(), nil, &res)
	out := agent.History{}
	if len(res.Data) == 50 {
		out.Previous = res.Cursor.Next
	}
	slices.Reverse(res.Data)
	for _, item := range res.Data {
		switch item.Type {
		case "user", "assistant", "idle":
			out.Messages = append(out.Messages, item.value())
		}
	}
	return out, err
}

func (p *Process) Interactions(ctx context.Context, id string) (agent.Interactions, error) {
	var permissions response[[]agent.Permission]
	var forms response[[]agent.Form]
	var permissionErr, formErr error
	var wg sync.WaitGroup
	wg.Go(func() { permissionErr = p.call(ctx, "GET", sessionPath(id, "/permission"), nil, &permissions) })
	wg.Go(func() { formErr = p.call(ctx, "GET", sessionPath(id, "/form"), nil, &forms) })
	wg.Wait()
	return agent.Interactions{Permissions: permissions.Data, Forms: forms.Data}, errors.Join(permissionErr, formErr)
}

func (p *Process) Decide(ctx context.Context, id, request, decision string) error {
	if !slices.Contains([]string{"once", "always", "reject"}, decision) {
		return errors.New("invalid permission decision")
	}
	return p.call(ctx, "POST", sessionPath(id, "/permission/"+url.PathEscape(request)+"/reply"), map[string]string{"decision": decision}, nil)
}

func (p *Process) Answer(ctx context.Context, id, form string, answer map[string]any) error {
	path := sessionPath(id, "/form/"+url.PathEscape(form))
	if answer == nil {
		return p.call(ctx, "DELETE", path, nil, nil)
	}
	return p.call(ctx, "POST", path+"/reply", map[string]any{"answer": answer}, nil)
}

func (p *Process) Diffs(ctx context.Context, id string) ([]agent.Diff, error) {
	var res response[[]agent.Diff]
	err := p.call(ctx, "GET", sessionPath(id, "/diff"), nil, &res)
	return res.Data, err
}

func (p *Process) Watch(ctx context.Context, emit func(agent.Event)) error {
	client, err := p.client(ctx)
	if err != nil {
		return err
	}
	res, err := client.request(ctx, "GET", "/api/event", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	lines := bufio.NewScanner(res.Body)
	lines.Buffer(make([]byte, 64<<10), 4<<20)
	var data []byte
	for lines.Scan() {
		line := lines.Bytes()
		if len(line) == 0 {
			if event, ok := eventFrom(data); ok {
				emit(event)
			}
			data = data[:0]
			continue
		}
		if content, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			data = append(data, bytes.TrimSpace(content)...)
			data = append(data, '\n')
		}
	}
	return lines.Err()
}

func eventFrom(data []byte) (agent.Event, bool) {
	if len(data) == 0 {
		return agent.Event{}, false
	}
	var wire struct {
		Type     string `json:"type"`
		Location struct {
			Directory string `json:"directory"`
		} `json:"location"`
		Data struct {
			SessionID string `json:"sessionID"`
			MessageID string `json:"assistantMessageID"`
			Ordinal   int    `json:"ordinal"`
			Delta     string `json:"delta"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return agent.Event{}, false
	}
	event := agent.Event{Kind: "change", SessionID: wire.Data.SessionID}
	switch wire.Type {
	case "server.connected":
		event.Kind = "connected"
	case "mcp.status.changed", "mcp.resources.changed":
		event.Kind = "catalog"
		event.Directory = wire.Location.Directory
	case "session.text.delta", "session.reasoning.delta":
		part, _, _ := strings.Cut(strings.TrimPrefix(wire.Type, "session."), ".")
		event = agent.Event{Kind: "delta", Part: part, SessionID: wire.Data.SessionID,
			MessageID: wire.Data.MessageID, Delta: wire.Data.Delta, Ordinal: wire.Data.Ordinal}
	case "session.execution.succeeded", "session.execution.failed", "session.execution.interrupted":
		event.Kind = "settled"
	default:
		if !strings.HasPrefix(wire.Type, "session.") && !strings.HasPrefix(wire.Type, "permission.") && !strings.HasPrefix(wire.Type, "form.") {
			return agent.Event{}, false
		}
	}
	return event, true
}

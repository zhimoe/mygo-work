// Package pi adapts the Pi Coding Agent CLI's JSONL RPC mode to MyGo Work.
package pi

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"mygo-work/internal/agent"
	"mygo-work/internal/localfile"
)

var _ agent.Runtime = (*Runtime)(nil)

var versionPattern = regexp.MustCompile(`(?m)^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.+-]+)?$`)

type Runtime struct {
	dir    string
	ctx    context.Context
	cancel context.CancelFunc
	events chan agent.Event

	mu          sync.Mutex
	sessions    map[string]sessionRecord
	connections map[string]*connection
	closed      bool
}

type sessionRecord struct {
	Session agent.Session `json:"session"`
	File    string        `json:"file,omitempty"`
}

type model struct {
	ID        string             `json:"id"`
	Provider  string             `json:"provider"`
	Name      string             `json:"name"`
	Reasoning bool               `json:"reasoning"`
	Type      string             `json:"type"`
	Thinking  map[string]*string `json:"thinkingLevelMap"`
	Input     []string           `json:"input"`
}

type remoteState struct {
	ID       string `json:"sessionId"`
	File     string `json:"sessionFile"`
	Model    model  `json:"model"`
	Thinking string `json:"thinkingLevel"`
}

func Open(directory string) (*Runtime, error) {
	dir := filepath.Join(directory, "pi")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	sessions := make(map[string]sessionRecord)
	raw, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &sessions); err != nil {
			return nil, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{dir: dir, ctx: ctx, cancel: cancel, events: make(chan agent.Event, 128),
		sessions: sessions, connections: make(map[string]*connection)}, nil
}

func (r *Runtime) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	connections := r.connections
	r.connections = nil
	r.mu.Unlock()
	var wait sync.WaitGroup
	for _, conn := range connections {
		wait.Go(conn.close)
	}
	wait.Wait()
	r.cancel()
}

func (r *Runtime) Version(ctx context.Context) (string, error) {
	launcher, err := executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := launcher.command(ctx, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("start Pi: %w", err)
	}
	version := versionPattern.FindString(strings.ReplaceAll(string(out), "\r\n", "\n"))
	if version == "" {
		return "", errors.New("Pi did not report a version")
	}
	return version, nil
}

func (r *Runtime) Watch(ctx context.Context, emit func(agent.Event)) error {
	for {
		select {
		case event := <-r.events:
			emit(event)
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return r.ctx.Err()
		}
	}
}

func (r *Runtime) emit(id, directory, kind string) {
	select {
	case r.events <- agent.Event{Kind: kind, SessionID: id, Directory: directory}:
	default: // The renderer periodically refreshes snapshots as well.
	}
}

func (r *Runtime) Sessions(_ context.Context, directory string, archived bool, search string) ([]agent.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []agent.Session
	search = strings.ToLower(strings.TrimSpace(search))
	for _, entry := range r.sessions {
		session := entry.Session
		if session.Directory == directory && session.Archived == archived && strings.Contains(strings.ToLower(session.Title), search) {
			result = append(result, session)
		}
	}
	slices.SortFunc(result, func(a, b agent.Session) int { return cmp.Compare(b.Updated, a.Updated) })
	return result, nil
}

func (r *Runtime) Create(ctx context.Context, directory string, prompt agent.Prompt) (agent.Session, error) {
	id := uuid.NewV4().String()
	entry := sessionRecord{Session: agent.Session{
		ID: id, Directory: directory, Updated: time.Now().UnixMilli(), Mode: "pi",
	}}
	r.mu.Lock()
	conn, err := r.connectLocked(entry)
	r.mu.Unlock()
	if err != nil {
		return agent.Session{}, err
	}
	var remote remoteState
	if err := conn.call(ctx, "get_state", nil, &remote); err != nil {
		r.discard(id, conn)
		return agent.Session{}, err
	}
	filename, err := r.sessionFile(entry, remote)
	if err != nil {
		r.discard(id, conn)
		return agent.Session{}, err
	}
	entry.File = filename
	if err := conn.choose(ctx, &remote, prompt.Model); err != nil {
		r.discard(id, conn)
		return agent.Session{}, err
	}
	if remote.Model.Provider != "unknown" {
		entry.Session.Model = remote.Model.ref(remote.Thinking)
	}
	r.mu.Lock()
	err = r.saveLocked(entry)
	r.mu.Unlock()
	if err != nil {
		r.discard(id, conn)
	}
	return entry.Session, err
}

func (r *Runtime) Update(_ context.Context, id string, patch agent.SessionPatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.sessions[id]
	if !ok {
		return errors.New("Pi session not found")
	}
	if title := strings.TrimSpace(patch.Title); title != "" {
		entry.Session.Title = title
	}
	if patch.Archived != nil {
		entry.Session.Archived = *patch.Archived
	}
	entry.Session.Updated = time.Now().UnixMilli()
	return r.saveLocked(entry)
}

func (r *Runtime) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.sessions[id]
	if !ok {
		return errors.New("Pi session not found")
	}
	if conn := r.connections[id]; conn != nil {
		conn.close()
		delete(r.connections, id)
	}
	delete(r.sessions, id)
	if err := r.writeIndexLocked(); err != nil {
		r.sessions[id] = entry
		return err
	}
	if filename := r.path(entry); filename != "" {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (r *Runtime) Options(ctx context.Context, directory string) (agent.Options, error) {
	lookup, cancel := context.WithCancel(r.ctx)
	defer cancel()
	conn, err := startRPC(lookup, directory, io.Discard, nil, "--no-session")
	if err != nil {
		return agent.Options{}, err
	}
	defer conn.close()
	var state remoteState
	if err := conn.call(ctx, "get_state", nil, &state); err != nil {
		return agent.Options{}, err
	}
	var models struct {
		Models []model `json:"models"`
	}
	if err := conn.call(ctx, "get_available_models", nil, &models); err != nil {
		return agent.Options{}, err
	}
	out := agent.Options{Modes: []agent.Mode{{ID: "pi", Name: "Pi"}}}
	if state.Model.Provider != "unknown" {
		out.DefaultModel = state.Model.ref(state.Thinking)
	}
	for _, model := range models.Models {
		if (model.Type != "" && model.Type != "chat") || !slices.Contains(model.Input, "text") {
			continue
		}
		out.Models = append(out.Models, agent.Model{ModelRef: model.ref(""), Name: model.Name, Variants: model.thinkingLevels()})
	}
	return out, nil
}

func (m model) thinkingLevels() []string {
	if !m.Reasoning {
		return nil
	}
	var levels []string
	for _, level := range []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"} {
		mapped, defined := m.Thinking[level]
		if defined && mapped == nil || !defined && (level == "xhigh" || level == "max") {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}

func (m model) ref(thinking string) agent.ModelRef {
	return agent.ModelRef{Provider: m.Provider, ID: m.ID, Variant: thinking}
}

func (r *Runtime) Active(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var active []string
	for id, conn := range r.connections {
		if conn.busy.Load() {
			active = append(active, id)
		}
	}
	return active, nil
}

func (r *Runtime) Prompt(ctx context.Context, id string, prompt agent.Prompt) error {
	entry, conn, err := r.get(id)
	if err != nil {
		return err
	}
	conn.submitMu.Lock()
	defer conn.submitMu.Unlock()
	if conn.busy.Load() {
		return errors.New("a Pi task is already running")
	}

	var state remoteState
	if err := conn.call(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	filename, err := r.sessionFile(entry, state)
	if err != nil {
		return err
	}
	if err := conn.choose(ctx, &state, prompt.Model); err != nil {
		return err
	}

	text, err := promptText(prompt)
	if err != nil {
		return err
	}
	r.mu.Lock()
	current, exists := r.sessions[id]
	if !exists {
		r.mu.Unlock()
		return errors.New("Pi session not found")
	}
	current.File = filename
	current.Session.Model = state.Model.ref(state.Thinking)
	if current.Session.Title == "" {
		title := strings.Join(strings.Fields(prompt.Text), " ")
		runes := []rune(title)
		if len(runes) > 58 {
			title = string(runes[:58]) + "…"
		}
		current.Session.Title = title
	}
	current.Session.Updated = time.Now().UnixMilli()
	err = r.saveLocked(current)
	r.mu.Unlock()
	if err != nil {
		return err
	}

	conn.busy.Store(true)
	var receipt struct {
		Disposition string `json:"disposition"`
	}
	err = conn.call(ctx, "prompt", map[string]any{"message": text}, &receipt)
	if err != nil || receipt.Disposition == "handled" {
		conn.busy.Store(false)
	}
	return err
}

func (conn *connection) choose(ctx context.Context, state *remoteState, choice agent.ModelRef) error {
	changedModel := choice.ID != "" && (state.Model.ID != choice.ID || state.Model.Provider != choice.Provider)
	if changedModel {
		if err := conn.call(ctx, "set_model", map[string]any{"provider": choice.Provider, "modelId": choice.ID}, nil); err != nil {
			return err
		}
	}
	changedThinking := choice.Variant != "" && (changedModel || state.Thinking != choice.Variant)
	if changedThinking {
		if err := conn.call(ctx, "set_thinking_level", map[string]any{"level": choice.Variant}, nil); err != nil {
			return err
		}
	}
	if changedModel || changedThinking {
		return conn.call(ctx, "get_state", nil, state)
	}
	return nil
}

func promptText(prompt agent.Prompt) (string, error) {
	if len(prompt.Skills) > 0 {
		return "", errors.New("Pi skills are configured by Pi")
	}
	var text strings.Builder
	text.WriteString(prompt.Text)
	if len(prompt.Files) > 0 {
		text.WriteString("\n\nSelected local files:\n")
		for _, file := range prompt.Files {
			path, err := localfile.Path(file.URI)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&text, "- %q\n", path)
		}
	}
	return text.String(), nil
}

func (r *Runtime) Interrupt(ctx context.Context, id string) error {
	r.mu.Lock()
	conn := r.connections[id]
	r.mu.Unlock()
	if conn == nil || !conn.busy.Load() {
		return nil
	}
	for _, form := range conn.formsSnapshot() {
		_ = conn.respond(ctx, form.ID, nil)
	}
	return conn.call(ctx, "abort", nil, nil)
}

func (r *Runtime) Diffs(context.Context, string) ([]agent.Diff, error) { return nil, nil }

func (r *Runtime) get(id string) (sessionRecord, *connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.sessions[id]
	if !ok {
		return sessionRecord{}, nil, errors.New("Pi session not found")
	}
	conn, err := r.connectLocked(entry)
	return entry, conn, err
}

func (r *Runtime) storage(directory string) string {
	return filepath.Join(r.dir, fmt.Sprintf("%x", sha256.Sum256([]byte(directory))))
}

func (r *Runtime) path(entry sessionRecord) string {
	if entry.File == "" || filepath.Base(entry.File) != entry.File {
		return ""
	}
	return filepath.Join(r.storage(entry.Session.Directory), entry.File)
}

func (r *Runtime) sessionFile(entry sessionRecord, remote remoteState) (string, error) {
	workspacePath := r.storage(entry.Session.Directory)
	if remote.ID != entry.Session.ID || remote.File == "" || filepath.Clean(filepath.Dir(remote.File)) != filepath.Clean(workspacePath) {
		return "", errors.New("Pi opened an unexpected session")
	}
	return filepath.Base(remote.File), nil
}

func (r *Runtime) connectLocked(entry sessionRecord) (*connection, error) {
	if r.closed {
		return nil, errors.New("Pi runtime is closed")
	}
	id, dir := entry.Session.ID, entry.Session.Directory
	if conn := r.connections[id]; conn != nil {
		select {
		case <-conn.done:
			delete(r.connections, id)
		default:
			return conn, nil
		}
	}
	store := r.storage(dir)
	if err := os.MkdirAll(store, 0700); err != nil {
		return nil, err
	}
	stderr, err := os.OpenFile(filepath.Join(store, id+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	args := []string{"--session-dir", store, "--session-id", id}
	if path := r.path(entry); path != "" {
		if _, err := os.Stat(path); err == nil {
			args = []string{"--session-dir", store, "--session", path}
		} else if !errors.Is(err, os.ErrNotExist) {
			stderr.Close()
			return nil, err
		}
	}
	conn, err := startRPC(r.ctx, dir, stderr, func(event record) {
		switch event.Type {
		case "agent_settled":
			r.emit(id, dir, "settled")
		case "agent_start", "message_start", "message_update", "message_end", "tool_execution_start", "tool_execution_end", "extension_ui_request":
			r.emit(id, dir, "change")
		}
	}, args...)
	if err != nil {
		stderr.Close()
		return nil, err
	}
	r.connections[id] = conn
	go func() {
		<-conn.done
		stderr.Close()
		r.emit(id, dir, "settled")
	}()
	return conn, nil
}

func (r *Runtime) discard(id string, conn *connection) {
	r.mu.Lock()
	if r.connections[id] == conn {
		delete(r.connections, id)
	}
	r.mu.Unlock()
	conn.close()
}

func (r *Runtime) saveLocked(entry sessionRecord) error {
	id := entry.Session.ID
	old, exists := r.sessions[id]
	r.sessions[id] = entry
	if err := r.writeIndexLocked(); err != nil {
		if exists {
			r.sessions[id] = old
		} else {
			delete(r.sessions, id)
		}
		return err
	}
	return nil
}

func (r *Runtime) writeIndexLocked() error {
	file, err := os.CreateTemp(r.dir, ".sessions-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := errors.Join(json.MarshalWrite(file, r.sessions), file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(r.dir, "sessions.json"))
}

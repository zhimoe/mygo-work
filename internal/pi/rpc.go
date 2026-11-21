package pi

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type record struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Success     bool            `json:"success"`
	Error       string          `json:"error"`
	Data        jsontext.Value  `json:"data"`
	Message     jsontext.Value  `json:"message"`
	Update      assistantUpdate `json:"assistantMessageEvent"`
	Method      string          `json:"method"`
	Title       string          `json:"title"`
	Options     []string        `json:"options"`
	Placeholder string          `json:"placeholder"`
	Prefill     string          `json:"prefill"`
	Timeout     int64           `json:"timeout"`
}

type assistantUpdate struct {
	Type     string   `json:"type"`
	Index    int      `json:"contentIndex"`
	Delta    string   `json:"delta"`
	Content  string   `json:"content"`
	ID       string   `json:"id"`
	ToolName string   `json:"toolName"`
	ToolCall wirePart `json:"toolCall"`
}

type connection struct {
	input io.WriteCloser
	cmd   *exec.Cmd
	done  chan struct{}

	sequence atomic.Uint64
	writeMu  sync.Mutex
	mu       sync.Mutex
	pending  map[string]chan record
	preview  *wireMessage
	forms    map[string]pendingForm
	busy     atomic.Bool
	submitMu sync.Mutex
}

func startRPC(ctx context.Context, directory string, stderr io.Writer, notify func(record), args ...string) (*connection, error) {
	launcher, err := executable()
	if err != nil {
		return nil, err
	}
	cmd := launcher.command(ctx, append([]string{"--mode", "rpc"}, args...)...)
	cmd.Dir = directory
	cmd.Stderr = stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	conn := &connection{input: input, cmd: cmd, done: make(chan struct{}), pending: make(map[string]chan record), forms: make(map[string]pendingForm)}
	go func() {
		defer close(conn.done)
		defer cmd.Wait()
		defer conn.busy.Store(false)
		scan := bufio.NewScanner(output)
		scan.Buffer(make([]byte, 64<<10), 64<<20)
		for scan.Scan() {
			var current record
			if json.Unmarshal(scan.Bytes(), &current) != nil {
				continue
			}
			if current.Type == "response" {
				conn.deliver(current)
			} else if notify != nil {
				conn.observe(current)
				notify(current)
			}
		}
		if scan.Err() != nil {
			_ = cmd.Cancel()
		}
	}()
	return conn, nil
}

func (conn *connection) deliver(reply record) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if wait := conn.pending[reply.ID]; wait != nil {
		select {
		case wait <- reply:
		default:
		}
	}
}

func (conn *connection) observe(event record) {
	switch event.Type {
	case "agent_start":
		conn.busy.Store(true)
	case "agent_settled":
		conn.busy.Store(false)
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	switch event.Type {
	case "message_start":
		var message wireMessage
		if len(event.Message) > 0 && json.Unmarshal(event.Message, &message) == nil && message.Role == "assistant" {
			message.Partial = message.parts()
			conn.preview = &message
		}
	case "message_update":
		if conn.preview != nil {
			conn.preview.apply(event.Update)
		}
	case "message_end":
		var message wireMessage
		if len(event.Message) > 0 && json.Unmarshal(event.Message, &message) == nil && message.Role == "assistant" {
			conn.preview = nil
		}
	case "agent_settled":
		conn.preview = nil
		clear(conn.forms)
	case "extension_ui_request":
		if form, ok := event.form(); ok {
			conn.forms[event.ID] = form
		}
	}
}

func (conn *connection) call(ctx context.Context, action string, args map[string]any, out any) error {
	if action != "prompt" {
		timeout, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		ctx = timeout
	}
	id := strconv.FormatUint(conn.sequence.Add(1), 10)
	if args == nil {
		args = make(map[string]any, 2)
	}
	args["id"], args["type"] = id, action
	wait := make(chan record, 1)
	conn.mu.Lock()
	conn.pending[id] = wait
	conn.mu.Unlock()
	defer func() {
		conn.mu.Lock()
		delete(conn.pending, id)
		conn.mu.Unlock()
	}()

	if err := conn.write(ctx, args); err != nil {
		return fmt.Errorf("send Pi RPC %s: %w", action, err)
	}
	select {
	case response := <-wait:
		if !response.Success {
			return fmt.Errorf("Pi %s: %s", action, strings.TrimSpace(response.Error))
		}
		if out != nil && len(response.Data) > 0 {
			return json.Unmarshal(response.Data, out)
		}
		return nil
	case <-conn.done:
		return errors.New("Pi process exited")
	case <-ctx.Done():
		return fmt.Errorf("waiting for Pi %s: %w", action, ctx.Err())
	}
}

func (conn *connection) write(ctx context.Context, args map[string]any) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = conn.input.Write(append(payload, '\n'))
	return err
}

func (conn *connection) close() {
	_ = conn.input.Close()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-conn.done:
	case <-timer.C:
		_ = conn.cmd.Cancel()
		<-conn.done
	}
}

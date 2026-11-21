package opencode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const TestedVersion = "2.3.2"

var v2Version = regexp.MustCompile(`^(opencode v?)?2\.[0-9]+\.[0-9]+[^\s]*$`)

type Process struct {
	ctx      context.Context
	cancel   context.CancelFunc
	dir      string
	mu       sync.Mutex
	running  *child
	mcpMu    sync.Mutex
	mcpReady map[string]string
}

type child struct {
	url  string
	pass string
	cmd  *exec.Cmd
	done chan struct{}
}

func NewProcess(dir string) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	return &Process{ctx: ctx, cancel: cancel, dir: dir, mcpReady: make(map[string]string)}
}

func (p *Process) Close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running != nil {
		<-p.running.done
	}
}

func (p *Process) client(ctx context.Context) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if p.running != nil {
		select {
		case <-p.running.done:
			p.running = nil
		default:
			return &Client{base: p.running.url, pass: p.running.pass}, nil
		}
	}
	startCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()

	bin, err := binary(startCtx, p.dir)
	if err != nil {
		return nil, err
	}
	server, err := p.start(startCtx, bin)
	if err != nil {
		return nil, err
	}
	client := &Client{base: server.url, pass: server.pass}
	if _, err := client.Version(startCtx); err != nil {
		_ = server.cmd.Cancel()
		<-server.done
		return nil, err
	}
	p.running = server
	return client, nil
}

func binary(ctx context.Context, dir string) (string, error) {
	bin := os.Getenv("MYGO_WORK_OPENCODE")
	candidates := []string{bin, filepath.Join(dir, "bin", "opencode")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "Resources", "opencode"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".opencode", "bin", "opencode"))
	}
	candidates = append(candidates, "opencode2", "opencode", "/opt/homebrew/bin/opencode", "/usr/local/bin/opencode")
	if bin != "" {
		candidates = []string{bin}
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		path, err := exec.LookPath(path)
		if err != nil {
			continue
		}
		output, err := exec.CommandContext(ctx, path, "--version").Output()
		if err == nil && v2Version.Match(bytes.TrimSpace(output)) {
			return filepath.Abs(path)
		}
	}
	return "", fmt.Errorf("OpenCode v2 executable unavailable. Install with scripts/install-opencode.sh or set MYGO_WORK_OPENCODE (tested with %s)", TestedVersion)
}

func (p *Process) start(ctx context.Context, bin string) (*child, error) {
	root := filepath.Join(p.dir, "engine")
	env := make(map[string]string)
	for _, key := range []string{"DATA", "STATE", "CACHE"} {
		path := filepath.Join(root, strings.ToLower(key))
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
		env["XDG_"+key+"_HOME"] = path
	}
	logFile, err := os.OpenFile(filepath.Join(root, "opencode.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	var secret [24]byte
	rand.Read(secret[:])
	server := &child{pass: hex.EncodeToString(secret[:]), done: make(chan struct{})}
	env["OPENCODE_PASSWORD"] = server.pass
	cmd := exec.CommandContext(p.ctx, bin, "serve", "--hostname", "127.0.0.1", "--port", "0")
	server.cmd = cmd
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "OPENCODE_SERVER_PASSWORD=")
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.Stderr = logFile
	output, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, err
	}
	ready := make(chan string, 1)
	go func() {
		defer close(server.done)
		defer logFile.Close()
		defer cmd.Wait()
		lines := bufio.NewReader(output)
		for {
			line, err := lines.ReadString('\n')
			if line != "" {
				fmt.Fprint(logFile, line)
			}
			if address, ok := strings.CutPrefix(strings.TrimSpace(line), "server listening on "); ok {
				u, parseErr := url.Parse(address)
				if parseErr == nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" {
					select {
					case ready <- u.String():
					default:
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()
	select {
	case server.url = <-ready:
		return server, nil
	case <-server.done:
		return nil, fmt.Errorf("OpenCode exited during startup. See %s", logFile.Name())
	case <-ctx.Done():
		_ = cmd.Cancel()
		<-server.done
		return nil, fmt.Errorf("waiting for OpenCode: %w", ctx.Err())
	}
}

type Client struct {
	base string
	pass string
}

var transport = &http.Transport{
	ResponseHeaderTimeout: 25 * time.Second,
	MaxIdleConnsPerHost:   12,
	IdleConnTimeout:       60 * time.Second,
}

var httpClient = &http.Client{
	Transport: transport,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", c.pass)
	req.Header.Set("Accept", "application/json")
	if path == "/api/event" {
		req.Header.Set("Accept", "text/event-stream")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("OpenCode connection interrupted")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		return nil, readError(method, path, res)
	}
	return res, nil
}

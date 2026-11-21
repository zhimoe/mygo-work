package opencode

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type remoteMCP struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Disabled bool   `json:"disabled,omitzero"`
}

func (p *Process) connectionStore() (map[string]map[string]remoteMCP, error) {
	result := make(map[string]map[string]remoteMCP)
	raw, err := os.ReadFile(filepath.Join(p.dir, "connections.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	return result, json.Unmarshal(raw, &result)
}

func (p *Process) saveConnections(connections map[string]map[string]remoteMCP) error {
	f, err := os.CreateTemp(p.dir, ".connections-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = errors.Join(json.MarshalWrite(f, connections), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(p.dir, "connections.json"))
}

func (p *Process) prepareConnections(ctx context.Context, directory string) error {
	p.mcpMu.Lock()
	defer p.mcpMu.Unlock()
	client, err := p.client(ctx)
	if err != nil {
		return err
	}
	if p.mcpReady[directory] == client.base {
		return nil
	}
	stored, err := p.connectionStore()
	if err != nil {
		return err
	}
	for _, item := range stored[directory] {
		if err := client.putMCP(ctx, directory, item); err != nil {
			return err
		}
	}
	p.mcpReady[directory] = client.base
	return nil
}

func (c *Client) putMCP(ctx context.Context, dir string, item remoteMCP) error {
	path := scoped("/api/experimental/mcp/"+url.PathEscape(item.Name), dir)
	return c.call(ctx, "PUT", path, map[string]any{
		"config": map[string]any{"type": "remote", "url": item.URL, "disabled": item.Disabled},
	}, nil)
}

func (p *Process) AddMCP(ctx context.Context, dir, name, address string) error {
	u, err := url.ParseRequestURI(strings.TrimSpace(address))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return errors.New("enter an HTTP or HTTPS MCP endpoint")
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "/\\") {
		return errors.New("enter a connection name without slashes")
	}
	p.mcpMu.Lock()
	defer p.mcpMu.Unlock()
	stored, err := p.connectionStore()
	if err != nil {
		return err
	}
	items := stored[dir]
	if items == nil {
		items = make(map[string]remoteMCP)
		stored[dir] = items
	}
	client, err := p.client(ctx)
	if err != nil {
		return err
	}
	item := remoteMCP{Name: name, URL: u.String()}
	if err := client.putMCP(ctx, dir, item); err != nil {
		return err
	}
	items[name] = item
	return p.saveConnections(stored)
}

func (p *Process) ConfigureMCP(ctx context.Context, dir, name, action string) error {
	p.mcpMu.Lock()
	defer p.mcpMu.Unlock()
	stored, err := p.connectionStore()
	if err != nil {
		return err
	}
	item, managed := stored[dir][name]
	path := "/api/experimental/mcp/" + url.PathEscape(name)
	switch action {
	case "connect", "disconnect":
		if err := p.call(ctx, "POST", scoped(path+"/"+action, dir), nil, nil); err != nil {
			return err
		}
		if !managed {
			return nil
		}
		item.Disabled = action == "disconnect"
		stored[dir][name] = item
	case "remove":
		if !managed {
			return errors.New("remove this connection from its OpenCode configuration")
		}
		if err := p.call(ctx, "DELETE", scoped(path, dir), nil, nil); err != nil {
			return err
		}
		delete(stored[dir], name)
	default:
		return errors.New("unknown connection action")
	}
	return p.saveConnections(stored)
}

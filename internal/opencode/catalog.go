package opencode

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type Connection struct {
	Name    string `json:"name"`
	Managed bool   `json:"managed"`
	Title   string `json:"title"`
	Status  struct {
		Status string `json:"status"`
		Type   string `json:"type"`
		Error  string `json:"error"`
	} `json:"status"`
}

type Provider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Catalog struct {
	Skills      []Skill      `json:"skills"`
	Connections []Connection `json:"connections"`
	Providers   []Provider   `json:"providers"`
}

func (p *Process) Catalog(ctx context.Context, directory string) (Catalog, error) {
	if err := p.prepareConnections(ctx, directory); err != nil {
		return Catalog{}, err
	}
	var skills response[[]Skill]
	var mcp response[[]Connection]
	var providers response[[]Provider]
	var skillErr, mcpErr, providerErr error
	var wg sync.WaitGroup
	wg.Go(func() { skillErr = p.call(ctx, "GET", scoped("/api/skill", directory), nil, &skills) })
	wg.Go(func() { mcpErr = p.call(ctx, "GET", scoped("/api/mcp", directory), nil, &mcp) })
	wg.Go(func() { providerErr = p.call(ctx, "GET", scoped("/api/provider", directory), nil, &providers) })
	wg.Wait()
	p.mcpMu.Lock()
	stored, storeErr := p.connectionStore()
	p.mcpMu.Unlock()
	for i, item := range mcp.Data {
		_, mcp.Data[i].Managed = stored[directory][item.Name]
	}
	return Catalog{Skills: skills.Data, Connections: mcp.Data, Providers: providers.Data}, errors.Join(skillErr, mcpErr, providerErr, storeErr)
}

func (p *Process) Integrations(ctx context.Context, dir string) ([]Provider, error) {
	var res response[[]struct {
		Provider
		Methods []struct {
			Type string `json:"type"`
		} `json:"methods"`
	}]
	err := p.call(ctx, "GET", scoped("/api/integration", dir), nil, &res)
	var out []Provider
	for _, item := range res.Data {
		for _, method := range item.Methods {
			if method.Type == "key" {
				out = append(out, item.Provider)
				break
			}
		}
	}
	return out, err
}

func (p *Process) SaveKey(ctx context.Context, integration, key string) error {
	if strings.TrimSpace(integration) == "" || strings.TrimSpace(key) == "" {
		return errors.New("provider and API key are required")
	}
	return p.call(ctx, "POST", "/api/credential", map[string]any{
		"integrationID": integration, "activate": true,
		"value": map[string]string{"type": "key", "key": strings.TrimSpace(key)},
	}, nil)
}

package main

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"mygo-work/internal/agent"
	"mygo-work/internal/localfile"
	"mygo-work/internal/opencode"
	"mygo-work/internal/workspace"
)

type Desktop struct {
	folders  *workspace.Store
	code     *opencode.Process
	runtimes map[string]agent.Runtime
	prefs    *preferences
}

func (d *Desktop) runtime(directory string) (agent.Runtime, error) {
	folder, err := d.folders.Get(directory)
	if err != nil {
		return nil, err
	}
	runtime := d.runtimes[cmp.Or(folder.Runtime, openCodeID)]
	if runtime == nil {
		return nil, errors.New("agent runtime unavailable for this workspace")
	}
	return runtime, nil
}

func (d *Desktop) Workspaces() []workspace.Folder { return d.folders.List() }

// defaultRuntime prefers Pi for new workspaces and falls back to the bundled
// OpenCode when Pi is not installed or cannot start.
func (d *Desktop) defaultRuntime(ctx context.Context) string {
	if d.PiAvailable(ctx) {
		return piID
	}
	return openCodeID
}

func (d *Desktop) PickWorkspace(ctx context.Context) (*workspace.Folder, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: d.nativeLabel("添加工作区", "Add workspace"), ButtonLabel: d.nativeLabel("使用此文件夹", "Use Folder"),
		Directory: true, CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	folder, err := d.folders.Add(paths[0], d.defaultRuntime(ctx))
	return &folder, err
}

func (d *Desktop) RenameWorkspace(dir, name string) error { return d.folders.Rename(dir, name) }
func (d *Desktop) RemoveWorkspace(dir string) error       { return d.folders.Remove(dir) }

func (d *Desktop) PickAttachments(ctx context.Context, dir string) ([]agent.Attachment, error) {
	if _, err := d.folders.Get(dir); err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: d.nativeLabel("添加附件", "Attach files"), ButtonLabel: d.nativeLabel("添加", "Attach"),
		DefaultPath: dir, Multiple: true,
	})
	if err != nil {
		return nil, err
	}
	items := make([]agent.Attachment, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("only regular files can be attached")
		}
		items = append(items, agent.Attachment{
			Name: filepath.Base(path), URI: localfile.URL(path),
		})
	}
	return items, nil
}

func (d *Desktop) Files(dir, path string) ([]workspace.Entry, error) {
	return d.folders.Files(dir, path)
}

func (d *Desktop) Preview(dir, path string) (workspace.Preview, error) {
	return d.folders.Preview(dir, path)
}

func (d *Desktop) SaveFile(dir, path, revision, text string, rows [][]string) error {
	return d.folders.SaveFile(dir, path, revision, text, rows)
}

func (d *Desktop) Reveal(dir, path string) error {
	real, err := d.folders.Resolve(dir, path)
	if err != nil {
		return err
	}
	mygo.Shell.ShowItemInFolder(real)
	return nil
}

func (d *Desktop) OpenLink(address string) error {
	u, err := url.ParseRequestURI(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("unsupported link")
	}
	return mygo.Shell.OpenExternal(u.String())
}

func (d *Desktop) Version(ctx context.Context, directory string) (string, error) {
	if directory == "" {
		return d.runtimes[openCodeID].Version(ctx)
	}
	runtime, err := d.runtime(directory)
	if err != nil {
		return "", err
	}
	return runtime.Version(ctx)
}

func (d *Desktop) Platform() string { return runtime.GOOS }

func (d *Desktop) PiAvailable(ctx context.Context) bool {
	runtime := d.runtimes[piID]
	if runtime == nil {
		return false
	}
	_, err := runtime.Version(ctx)
	return err == nil
}

func (d *Desktop) SetWorkspaceRuntime(ctx context.Context, directory, id string) error {
	runtime := d.runtimes[id]
	if runtime == nil {
		return errors.New("unknown coding engine")
	}
	if _, err := runtime.Version(ctx); err != nil {
		return err
	}
	return d.folders.SetRuntime(directory, id)
}

func (d *Desktop) Sessions(ctx context.Context, dir string, archived bool, search string) ([]agent.Session, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return nil, err
	}
	return r.Sessions(ctx, dir, archived, search)
}

func (d *Desktop) Create(ctx context.Context, dir string, prompt agent.Prompt) (agent.Session, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return agent.Session{}, err
	}
	return r.Create(ctx, dir, prompt)
}

func (d *Desktop) Update(ctx context.Context, dir, id string, patch agent.SessionPatch) error {
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Update(ctx, id, patch)
}

func (d *Desktop) Delete(ctx context.Context, dir, id string) error {
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Delete(ctx, id)
}

func (d *Desktop) Options(ctx context.Context, dir string) (agent.Options, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return agent.Options{}, err
	}
	return r.Options(ctx, dir)
}

func (d *Desktop) History(ctx context.Context, dir, id, cursor string) (agent.History, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return agent.History{}, err
	}
	return r.History(ctx, id, cursor)
}

func (d *Desktop) Active(ctx context.Context, dir string) ([]string, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return nil, err
	}
	return r.Active(ctx)
}

func (d *Desktop) Prompt(ctx context.Context, dir, id string, prompt agent.Prompt) error {
	if strings.TrimSpace(prompt.Text) == "" && len(prompt.Files) == 0 && len(prompt.Skills) == 0 {
		return errors.New("enter a task or add an attachment")
	}
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Prompt(ctx, id, prompt)
}

func (d *Desktop) Interrupt(ctx context.Context, dir, id string) error {
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Interrupt(ctx, id)
}

func (d *Desktop) Interactions(ctx context.Context, dir, id string) (agent.Interactions, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return agent.Interactions{}, err
	}
	return r.Interactions(ctx, id)
}

func (d *Desktop) Decide(ctx context.Context, dir, id, request, decision string) error {
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Decide(ctx, id, request, decision)
}

func (d *Desktop) Answer(ctx context.Context, dir, id, form string, answer map[string]any) error {
	r, err := d.runtime(dir)
	if err != nil {
		return err
	}
	return r.Answer(ctx, id, form, answer)
}

func (d *Desktop) Diffs(ctx context.Context, dir, id string) ([]agent.Diff, error) {
	r, err := d.runtime(dir)
	if err != nil {
		return nil, err
	}
	return r.Diffs(ctx, id)
}

func (d *Desktop) Catalog(ctx context.Context, dir string) (opencode.Catalog, error) {
	if _, err := d.folders.Get(dir); err != nil {
		return opencode.Catalog{}, err
	}
	return d.code.Catalog(ctx, dir)
}

func (d *Desktop) Integrations(ctx context.Context, dir string) ([]opencode.Provider, error) {
	if _, err := d.folders.Get(dir); err != nil {
		return nil, err
	}
	return d.code.Integrations(ctx, dir)
}

func (d *Desktop) SaveKey(ctx context.Context, integration, key string) error {
	return d.code.SaveKey(ctx, integration, key)
}

func (d *Desktop) AddMCP(ctx context.Context, dir, name, address string) error {
	if _, err := d.folders.Get(dir); err != nil {
		return err
	}
	return d.code.AddMCP(ctx, dir, name, address)
}

func (d *Desktop) ConfigureMCP(ctx context.Context, dir, name, action string) error {
	if _, err := d.folders.Get(dir); err != nil {
		return err
	}
	return d.code.ConfigureMCP(ctx, dir, name, action)
}

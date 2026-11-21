// Package agent defines the session contract shared by desktop agent runtimes.
package agent

import "context"

type Session struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Directory string   `json:"directory"`
	Updated   int64    `json:"updated"`
	Archived  bool     `json:"archived"`
	Model     ModelRef `json:"model"`
	Mode      string   `json:"mode"`
}

type ModelRef struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Variant  string `json:"variant,omitempty"`
}

type Model struct {
	ModelRef
	Name     string   `json:"name"`
	Variants []string `json:"variants"`
}

type Mode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Options struct {
	Models       []Model  `json:"models"`
	DefaultModel ModelRef `json:"defaultModel"`
	Modes        []Mode   `json:"modes"`
}

type Attachment struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type Prompt struct {
	Text   string       `json:"text"`
	Files  []Attachment `json:"files"`
	Skills []string     `json:"skills"`
	Model  ModelRef     `json:"model"`
	Mode   string       `json:"mode"`
}

type Part struct {
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Input  any    `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

type Message struct {
	ID       string       `json:"id"`
	Role     string       `json:"role"`
	Parts    []Part       `json:"parts"`
	Files    []Attachment `json:"files,omitempty"`
	Outcome  string       `json:"outcome,omitempty"`
	Complete bool         `json:"complete,omitzero"`

	Error string `json:"error,omitempty"`
}

type History struct {
	Messages []Message `json:"messages"`
	Previous string    `json:"previous"`
}

type Permission struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
	Message   string   `json:"message"`
}

type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Condition struct {
	Op string `json:"op"`

	Key   string `json:"key"`
	Value any    `json:"value"`
}

type Field struct {
	Key         string      `json:"key"`
	Type        string      `json:"type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Hidden      bool        `json:"hidden"`
	Default     any         `json:"default"`
	When        []Condition `json:"when"`
	Options     []Choice    `json:"options"`
	Custom      bool        `json:"custom"`
}

type Form struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Fields []Field `json:"fields"`
}

type Interactions struct {
	Permissions []Permission `json:"permissions"`
	Forms       []Form       `json:"forms"`
}

type Diff struct {
	File      string `json:"file"`
	Patch     string `json:"patch"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// Event carries transcript fragments and invalidations. History remains the
// authority for completed messages and reconnects.
type Event struct {
	Kind      string `json:"kind"`
	Runtime   string `json:"runtime,omitempty"`
	Directory string `json:"directory,omitempty"`
	SessionID string `json:"sessionID,omitempty"`
	MessageID string `json:"messageID,omitempty"`
	Part      string `json:"part,omitempty"`
	Ordinal   int    `json:"ordinal,omitzero"`
	Delta     string `json:"delta,omitempty"`
}

type SessionPatch struct {
	Title    string `json:"title,omitempty"`
	Archived *bool  `json:"archived,omitempty"`
}

type Runtime interface {
	Version(context.Context) (string, error)
	Sessions(context.Context, string, bool, string) ([]Session, error)
	Create(context.Context, string, Prompt) (Session, error)
	Update(context.Context, string, SessionPatch) error
	Delete(context.Context, string) error
	Options(context.Context, string) (Options, error)
	History(context.Context, string, string) (History, error)
	Active(context.Context) ([]string, error)
	Prompt(context.Context, string, Prompt) error
	Interrupt(context.Context, string) error
	Interactions(context.Context, string) (Interactions, error)
	Decide(context.Context, string, string, string) error
	Answer(context.Context, string, string, map[string]any) error
	Diffs(context.Context, string) ([]Diff, error)
	Watch(context.Context, func(Event)) error
}

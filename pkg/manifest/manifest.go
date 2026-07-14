// Package manifest defines and validates orvalho.json package manifests.
//
// Pure library: JSON only. No zip IO, no network, no runtime.
package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Filename is the fixed path of the package manifest inside an Orvalho zip.
const Filename = "orvalho.json"

// SchemaVersionCurrent is the only schema_version accepted by this package.
const SchemaVersionCurrent = 1

// Runtime is the guest runtime kind declared in orvalho.json.
//
// encoding/json marshals and unmarshals this as a plain string.
type Runtime string

// Guest runtime kinds.
const (
	// RuntimeJS is the only guest runtime kind supported in v1.
	RuntimeJS Runtime = "js"
)

// ProtocolHTTP is the default publish protocol hint.
const ProtocolHTTP = "http"

// String returns the runtime kind as a string.
func (r Runtime) String() string {
	return string(r)
}

// Valid reports whether r is a known runtime kind for the current schema.
func (r Runtime) Valid() bool {
	switch r {
	case RuntimeJS:
		return true
	default:
		return false
	}
}

// Manifest is the parsed form of orvalho.json.
//
// It declares actor identity, entry, requested bindings, egress allowlist,
// and port/publish hints. IPv6 assignment is manager authority and is not
// part of the package manifest.
type Manifest struct {
	// SchemaVersion must be SchemaVersionCurrent (1).
	SchemaVersion int `json:"schema_version"`

	// ID is the actor identity within a mesh (stable package name).
	// DNS-label style: lowercase letter, then lowercase alphanumerics or hyphens.
	ID string `json:"id"`

	// Name is an optional human-readable display name.
	Name string `json:"name,omitempty"`

	// Entry is the package-relative path to the JS worker entry module.
	Entry string `json:"entry"`

	// Runtime is the guest runtime kind. v1: RuntimeJS ("js").
	Runtime Runtime `json:"runtime"`

	// Bindings declares host-injected env bindings requested by the package.
	Bindings *Bindings `json:"bindings,omitempty"`

	// Egress is the outbound fetch allowlist (host or origin patterns).
	// Empty means no outbound network (safe default).
	Egress []string `json:"egress,omitempty"`

	// Port is a preferred publish/listen port hint (1–65535). Optional.
	// Manager may override; IPv6 address remains manager-allocated.
	Port int `json:"port,omitempty"`

	// Publish holds optional publish-related hints.
	Publish *PublishHints `json:"publish,omitempty"`
}

// Bindings declares which env bindings the package expects.
//
// v1 families: assets, secrets, config. Device/storage bindings use the same
// shape later; unknown families are rejected by the decoder.
type Bindings struct {
	// Assets, when set, requests read-only package file access via env.
	Assets *AssetsBinding `json:"assets,omitempty"`

	// Secrets are named values injected by the manager at install (not in the zip).
	Secrets []NameBinding `json:"secrets,omitempty"`

	// Config are named non-secret configuration values injected at install.
	Config []NameBinding `json:"config,omitempty"`
}

// AssetsBinding describes package paths exposed as read-only assets.
// At least one of Root or Paths must be set.
type AssetsBinding struct {
	// Root is a package-relative directory root for assets.
	Root string `json:"root,omitempty"`

	// Paths lists individual package-relative files or directories.
	Paths []string `json:"paths,omitempty"`
}

// NameBinding declares a single named env binding (secret or config).
type NameBinding struct {
	// Name is the env key (e.g. API_KEY). Letter/underscore start, then alphanumerics/underscore.
	Name string `json:"name"`

	// Required, if true, means install must supply a value.
	Required bool `json:"required,omitempty"`
}

// PublishHints are optional hints for how the actor is published on the mesh.
// Address allocation is not declared here.
type PublishHints struct {
	// Port preferred for publish (1–65535). If 0, callers may fall back to Manifest.Port.
	Port int `json:"port,omitempty"`

	// Protocol is the publish protocol hint. v1: "http".
	Protocol string `json:"protocol,omitempty"`
}

// Parse decodes and validates an orvalho.json document.
// Unknown JSON fields are rejected. On success the returned Manifest is valid.
func Parse(data []byte) (*Manifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, &Error{Message: "empty document"}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, &Error{Message: fmt.Sprintf("parse: %v", err)}
	}

	// Reject trailing junk after the root value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, &Error{Message: "parse: trailing data after JSON value"}
		}
		return nil, &Error{Message: fmt.Sprintf("parse: trailing data: %v", err)}
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// MustParse is like Parse but panics on error. Intended for tests and fixtures.
func MustParse(data []byte) *Manifest {
	m, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return m
}

// PreferredPort returns the best port hint: publish.port, then port, else 0.
func (m *Manifest) PreferredPort() int {
	if m == nil {
		return 0
	}
	if m.Publish != nil && m.Publish.Port != 0 {
		return m.Publish.Port
	}
	return m.Port
}

// String returns a short description for logs.
func (m *Manifest) String() string {
	if m == nil {
		return "<nil manifest>"
	}
	return fmt.Sprintf("manifest{id=%q entry=%q runtime=%q}", m.ID, m.Entry, m.Runtime)
}

// Normalize trims obvious whitespace on string fields after parse.
// Called from Validate so callers of Parse get a cleaned manifest.
func (m *Manifest) normalize() {
	m.ID = strings.TrimSpace(m.ID)
	m.Name = strings.TrimSpace(m.Name)
	m.Entry = strings.TrimSpace(m.Entry)
	m.Runtime = Runtime(strings.TrimSpace(string(m.Runtime)))

	if m.Bindings != nil {
		if m.Bindings.Assets != nil {
			m.Bindings.Assets.Root = strings.TrimSpace(m.Bindings.Assets.Root)
			for i, p := range m.Bindings.Assets.Paths {
				m.Bindings.Assets.Paths[i] = strings.TrimSpace(p)
			}
		}
		for i := range m.Bindings.Secrets {
			m.Bindings.Secrets[i].Name = strings.TrimSpace(m.Bindings.Secrets[i].Name)
		}
		for i := range m.Bindings.Config {
			m.Bindings.Config[i].Name = strings.TrimSpace(m.Bindings.Config[i].Name)
		}
	}

	for i, e := range m.Egress {
		m.Egress[i] = strings.TrimSpace(e)
	}

	if m.Publish != nil {
		m.Publish.Protocol = strings.TrimSpace(m.Publish.Protocol)
	}
}

package dependency

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultRegistry = "https://registry.npmjs.org"

type packument struct {
	Name     string                      `json:"name"`
	DistTags map[string]string           `json:"dist-tags"`
	Versions map[string]packumentVersion `json:"versions"`
}

type packumentVersion struct {
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	CPU                  []string          `json:"cpu"`
	OS                   []string          `json:"os"`
	Bin                  any               `json:"bin"`
	Dist                 struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
		Shasum    string `json:"shasum"`
	} `json:"dist"`
}

type registry struct {
	base   string
	client *http.Client
}

func (r registry) url() string {
	if r.base != "" {
		return strings.TrimRight(r.base, "/")
	}
	return defaultRegistry
}

func (r registry) http() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

func (r registry) packument(name string) (*packument, error) {
	u, err := url.JoinPath(r.url(), strings.ReplaceAll(name, "/", "%2f"))
	if err != nil {
		return nil, err
	}
	// JoinPath decodes; scoped names need the encoded slash for npm.
	if strings.HasPrefix(name, "@") {
		u = r.url() + "/" + strings.ReplaceAll(name, "/", "%2f")
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRegistry, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, drainErr := io.Copy(io.Discard, resp.Body)
		return nil, errors.Join(fmt.Errorf("%w: %s: %s", ErrRegistry, name, resp.Status), drainErr)
	}
	var p packument
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRegistry, err)
	}
	return &p, nil
}

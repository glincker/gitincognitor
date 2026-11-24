package registry

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Store  string `yaml:"store"`
	Branch string `yaml:"branch"`
	Remote string `yaml:"remote,omitempty"`
}

type Registry struct {
	Version  int       `yaml:"version"`
	Projects []Project `yaml:"projects"`
}

func ExpandPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == "~" || strings.HasPrefix(p, "~/")) {
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func ShortPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func Load(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Registry{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Version == 0 {
		r.Version = 1
	}
	for i := range r.Projects {
		r.Projects[i].Path = ExpandPath(r.Projects[i].Path)
	}
	return &r, nil
}

func (r *Registry) Save(path string) error {
	out := Registry{Version: r.Version, Projects: make([]Project, len(r.Projects))}
	copy(out.Projects, r.Projects)
	for i := range out.Projects {
		out.Projects[i].Path = ShortPath(out.Projects[i].Path)
	}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (r *Registry) Find(name string) *Project {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			return &r.Projects[i]
		}
	}
	return nil
}

func (r *Registry) Upsert(p Project) {
	if e := r.Find(p.Name); e != nil {
		*e = p
		return
	}
	r.Projects = append(r.Projects, p)
}

func (r *Registry) Remove(name string) {
	out := r.Projects[:0]
	for _, p := range r.Projects {
		if p.Name != name {
			out = append(out, p)
		}
	}
	r.Projects = out
}

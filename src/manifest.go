package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Tool struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Category     string         `json:"category"`
	Kind         string         `json:"kind"`
	Availability string         `json:"availability"`
	License      string         `json:"license"`
	Description  string         `json:"description"`
	Homepage     string         `json:"homepage"`
	Exe          []string       `json:"exe"`
	ConsoleExe   []string       `json:"console_exe"`
	SearchDirs   []string       `json:"search_dirs"`
	Args         []string       `json:"args"`
	Hint         string         `json:"hint"`
	Warning      string         `json:"warning"`
	Alternative  string         `json:"alternative"`
	URL          string         `json:"url"`
	AuthRequired bool           `json:"auth_required"`
	Optional     bool           `json:"optional"`
	Fetch        map[string]any `json:"fetch"`
	Bundled      bool           `json:"bundled,omitempty"`
	Runtime      bool           `json:"runtime,omitempty"`      // componente di supporto (Python): solo negli aggiornamenti, non nel pannello
	Redistribute *bool          `json:"redistribute,omitempty"` // false = escluso dal pacchetto completo pubblicato
	Custom       bool           `json:"-"`
}

type Manifest struct {
	Schema     int      `json:"schema"`
	Notes      string   `json:"notes"`
	Categories []string `json:"categories"`
	Tools      []Tool   `json:"tools"`
}

func loadManifest(base string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(base, "manifest", "tools.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	// strumenti personali in coda, nelle loro categorie (anche nuove)
	for _, t := range loadCustom(base).Tools {
		if m.find(t.ID) != nil {
			continue
		}
		known := false
		for _, c := range m.Categories {
			known = known || c == t.Category
		}
		if !known {
			m.Categories = append(m.Categories, t.Category)
		}
		m.Tools = append(m.Tools, t)
	}
	return &m, nil
}

func (m *Manifest) find(id string) *Tool {
	for i := range m.Tools {
		if m.Tools[i].ID == id {
			return &m.Tools[i]
		}
	}
	return nil
}

// searchRoots: tools/<id> e le eventuali cartelle aggiuntive (es. runtime/bin per i tool Python).
func searchRoots(base string, t *Tool) []string {
	roots := []string{filepath.Join(base, "tools", t.ID)}
	for _, d := range t.SearchDirs {
		roots = append(roots, filepath.Join(base, filepath.FromSlash(d)))
	}
	return roots
}

// findExe cerca i file per nome (glob, senza distinzione di maiuscole) fino a 6 livelli di profondita'.
// L'ordine dei pattern e' la priorita'; a parita' vince il percorso piu' corto, poi l'ordine alfabetico.
func findExe(base string, t *Tool, patterns []string) string {
	for _, pat := range patterns {
		lp := strings.ToLower(pat)
		var hits []string
		for _, root := range searchRoots(base, t) {
			rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
			filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					if strings.Count(filepath.Clean(p), string(os.PathSeparator))-rootDepth >= 6 {
						return filepath.SkipDir
					}
					return nil
				}
				if ok, _ := filepath.Match(lp, strings.ToLower(d.Name())); ok {
					hits = append(hits, p)
				}
				return nil
			})
		}
		if len(hits) > 0 {
			sort.Slice(hits, func(i, j int) bool {
				if len(hits[i]) != len(hits[j]) {
					return len(hits[i]) < len(hits[j])
				}
				return hits[i] < hits[j]
			})
			return hits[0]
		}
	}
	return ""
}

func runnable(t *Tool) bool {
	switch t.Availability {
	case "linux", "deprecated":
		return false
	}
	return true
}

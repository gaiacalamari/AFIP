package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const genesis = "0000000000000000000000000000000000000000000000000000000000000000"

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// --- cache degli hash: ricalcola solo se dimensione o data di modifica cambiano ---------------
type hashCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}
type cacheEntry struct {
	size  int64
	mtime time.Time
	sum   string
}

func (c *hashCache) get(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	e, ok := c.m[path]
	c.mu.Unlock()
	if ok && e.size == st.Size() && e.mtime.Equal(st.ModTime()) {
		return e.sum, nil
	}
	sum, _, err := hashFile(path)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.m[path] = cacheEntry{st.Size(), st.ModTime(), sum}
	c.mu.Unlock()
	return sum, nil
}

// --- sigillo del toolkit ---------------------------------------------------------------------
type SealFile struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Seal struct {
	Created string              `json:"created"`
	Host    string              `json:"host"`
	User    string              `json:"user"`
	Count   int                 `json:"count"`
	Digest  string              `json:"digest"`
	Files   map[string]SealFile `json:"files"`
}

func sealPath(base string) string { return filepath.Join(base, "manifest", "sealed.json") }

func scanToolkit(base string) (map[string]SealFile, error) {
	files := map[string]SealFile{}
	for _, top := range []string{"tools", "runtime"} {
		root := filepath.Join(base, top)
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			sum, size, err := hashFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(base, p)
			files[filepath.ToSlash(rel)] = SealFile{sum, size}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func digestOf(files map[string]SealFile) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		io.WriteString(h, k+"\x00"+files[k].SHA256+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func createSeal(base, host, user string) (*Seal, error) {
	files, err := scanToolkit(base)
	if err != nil {
		return nil, err
	}
	s := &Seal{Created: time.Now().UTC().Format(time.RFC3339), Host: host, User: user, Count: len(files), Digest: digestOf(files), Files: files}
	b, _ := json.MarshalIndent(s, "", "  ")
	return s, os.WriteFile(sealPath(base), b, 0o644)
}

func loadSeal(base string) *Seal {
	b, err := os.ReadFile(sealPath(base))
	if err != nil {
		return nil
	}
	var s Seal
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

type ToolkitDiff struct {
	Sealed  bool     `json:"sealed"`
	Valid   bool     `json:"valid"`
	Checked int      `json:"checked"`
	Changed []string `json:"changed"`
	Missing []string `json:"missing"`
	Added   []string `json:"added"`
	Digest  string   `json:"digest"`
}

func verifyToolkit(base string) (*ToolkitDiff, error) {
	s := loadSeal(base)
	if s == nil {
		return &ToolkitDiff{Sealed: false}, nil
	}
	cur, err := scanToolkit(base)
	if err != nil {
		return nil, err
	}
	d := &ToolkitDiff{Sealed: true, Checked: len(cur), Changed: []string{}, Missing: []string{}, Added: []string{}, Digest: digestOf(cur)}
	for k, v := range s.Files {
		c, ok := cur[k]
		if !ok {
			d.Missing = append(d.Missing, k)
		} else if c.SHA256 != v.SHA256 {
			d.Changed = append(d.Changed, k)
		}
	}
	for k := range cur {
		if _, ok := s.Files[k]; !ok {
			d.Added = append(d.Added, k)
		}
	}
	sort.Strings(d.Changed)
	sort.Strings(d.Missing)
	sort.Strings(d.Added)
	d.Valid = len(d.Changed)+len(d.Missing)+len(d.Added) == 0
	return d, nil
}

// --- registro delle esecuzioni, append-only a catena di hash ----------------------------------
type LogEntry struct {
	Seq     int    `json:"seq"`
	TS      string `json:"ts"`
	Host    string `json:"host"`
	User    string `json:"user"`
	Action  string `json:"action"`
	Tool    string `json:"tool,omitempty"`
	Exe     string `json:"exe,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Case    string `json:"case,omitempty"` // mantenuto per compatibilita' con i registri della 0.1/1.0
	AuthRef string `json:"auth_ref,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Prev    string `json:"prev"`
	Hash    string `json:"hash,omitempty"`
}

func entryHash(e LogEntry) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type Ledger struct {
	mu   sync.Mutex
	path string
}

func (l *Ledger) readAll() ([]LogEntry, error) {
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []LogEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return out, errors.New("riga del registro non leggibile")
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (l *Ledger) Append(e LogEntry) (LogEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := l.readAll()
	if err != nil {
		return e, err
	}
	e.Seq, e.Prev = 1, genesis
	if n := len(all); n > 0 {
		e.Seq, e.Prev = all[n-1].Seq+1, all[n-1].Hash
	}
	e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	e.Hash = entryHash(e)
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return e, err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return e, err
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	if _, err := f.Write(append(b, '\n')); err != nil {
		return e, err
	}
	return e, f.Sync()
}

type ChainStatus struct {
	Valid    bool   `json:"valid"`
	Entries  int    `json:"entries"`
	Head     string `json:"head"`
	BrokenAt int    `json:"broken_at,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (l *Ledger) Verify() ChainStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := l.readAll()
	if err != nil {
		return ChainStatus{Valid: false, Entries: len(all), BrokenAt: len(all) + 1, Error: err.Error()}
	}
	prev := genesis
	for i, e := range all {
		if e.Seq != i+1 || e.Prev != prev || entryHash(e) != e.Hash {
			return ChainStatus{Valid: false, Entries: len(all), BrokenAt: i + 1}
		}
		prev = e.Hash
	}
	return ChainStatus{Valid: true, Entries: len(all), Head: prev}
}

func hashReader(s string) (string, int64, error) {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:]), int64(len(s)), nil
}

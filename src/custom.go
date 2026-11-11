package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const customCategory = "I miei strumenti"

// Strumenti personali: stanno in manifest/custom-tools.json, separati dal catalogo, cosi' gli aggiornamenti
// del catalogo non li toccano.
type CustomFile struct {
	Tools []Tool `json:"tools"`
}

func customPath(base string) string { return filepath.Join(base, "manifest", "custom-tools.json") }

func loadCustom(base string) CustomFile {
	var c CustomFile
	if b, err := os.ReadFile(customPath(base)); err == nil {
		json.Unmarshal(b, &c)
	}
	for i := range c.Tools {
		c.Tools[i].Custom = true
	}
	return c
}

func saveCustom(base string, c CustomFile) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(customPath(base), b, 0o644)
}

// peKind legge il sottosistema dall'intestazione PE: "gui" per i programmi a finestre, "cli" per quelli da console.
func peKind(path string) string {
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".cmd") || strings.HasSuffix(low, ".bat") || strings.HasSuffix(low, ".ps1") {
		return "cli"
	}
	if strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm") {
		return "gui"
	}
	f, err := os.Open(path)
	if err != nil {
		return "gui"
	}
	defer f.Close()
	hdr := make([]byte, 64)
	if _, err := io.ReadFull(f, hdr); err != nil || hdr[0] != 'M' || hdr[1] != 'Z' {
		return "gui"
	}
	off := int64(binary.LittleEndian.Uint32(hdr[0x3c:]))
	pe := make([]byte, 0x60)
	if _, err := f.ReadAt(pe, off); err != nil || string(pe[:4]) != "PE\x00\x00" {
		return "gui"
	}
	if binary.LittleEndian.Uint16(pe[0x5c:]) == 3 { // IMAGE_SUBSYSTEM_WINDOWS_CUI
		return "cli"
	}
	return "gui"
}

func dirSize(root string) (int64, int) {
	var total int64
	n := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
				n++
			}
		}
		return nil
	})
	return total, n
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		return copyFile(p, t)
	})
}

// AddCustomTool copia l'eseguibile (o la sua cartella) in tools/custom-<nome> e lo aggiunge agli strumenti personali.
func (a *App) AddCustomTool(exe, name, category, kind string, wholeFolder bool) (Tool, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 2 {
		return Tool{}, errors.New("nome troppo corto")
	}
	if category = strings.TrimSpace(category); category == "" {
		category = customCategory
	}
	if _, err := os.Stat(exe); err != nil {
		return Tool{}, err
	}
	if rel, err := filepath.Rel(a.Base, exe); err == nil && !strings.HasPrefix(rel, "..") {
		return Tool{}, errors.New("il file è già dentro la cartella di AFIP Portable")
	}
	if kind != "cli" && kind != "gui" {
		kind = peKind(exe)
	}
	if wholeFolder {
		dir := filepath.Dir(exe)
		if filepath.Dir(dir) == dir { // radice di un'unita' (C:\): mai copiarla intera
			return Tool{}, errors.New("il programma è nella radice di un'unità: copiare solo il file")
		}
	}
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.ReplaceAll(name, " ", "-")), ""), "-_")
	if slug == "" {
		slug = "strumento"
	}
	m, _ := loadManifest(a.Base)
	id := "custom-" + slug
	for i := 2; m != nil && m.find(id) != nil; i++ {
		id = fmt.Sprintf("custom-%s-%d", slug, i)
	}
	dest := filepath.Join(a.Base, "tools", id)
	var err error
	if wholeFolder {
		err = copyDir(filepath.Dir(exe), filepath.Join(dest, filepath.Base(filepath.Dir(exe))))
	} else {
		err = copyFile(exe, filepath.Join(dest, filepath.Base(exe)))
	}
	if err != nil {
		os.RemoveAll(dest)
		return Tool{}, fmt.Errorf("copia non riuscita: %w", err)
	}
	t := Tool{ID: id, Name: name, Category: category, Kind: kind, Availability: "portable", License: "aggiunto dall'utente",
		Description: "Strumento personale · " + filepath.Base(exe), Exe: []string{filepath.Base(exe)}, Custom: true}
	if kind == "cli" {
		t.Hint = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe)) + " --help"
	}
	c := loadCustom(a.Base)
	c.Tools = append(c.Tools, t)
	if err := saveCustom(a.Base, c); err != nil {
		return t, err
	}
	if err := a.resealPrefix("tools/" + id + "/"); err != nil {
		return t, err
	}
	sum, _, _ := hashFile(exe)
	what := "solo il file"
	if wholeFolder {
		what = "cartella intera"
	}
	a.log(LogEntry{Action: "custom.add", Tool: id, SHA256: sum, Detail: fmt.Sprintf("%s da %s (%s)", name, exe, what)})
	return t, nil
}

// RemoveCustomTool toglie lo strumento dagli strumenti personali e ne cancella la copia nella pendrive.
func (a *App) RemoveCustomTool(id string) error {
	if !strings.HasPrefix(id, "custom-") || slugRe.MatchString(id) {
		return errors.New("si possono rimuovere solo gli strumenti personali")
	}
	c := loadCustom(a.Base)
	kept := c.Tools[:0]
	found := false
	for _, t := range c.Tools {
		if t.ID == id {
			found = true
			continue
		}
		kept = append(kept, t)
	}
	if !found {
		return errors.New("strumento non trovato")
	}
	c.Tools = kept
	if err := saveCustom(a.Base, c); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(a.Base, "tools", id)); err != nil {
		return fmt.Errorf("rimosso dall'elenco, ma la cartella non si cancella (il programma è aperto?): %w", err)
	}
	a.resealPrefix("tools/" + id + "/")
	a.log(LogEntry{Action: "custom.remove", Tool: id})
	return nil
}

// --- adattatori usati dalla schermata degli aggiornamenti --------------------------------------

func (a *App) updatable(m *Manifest) []*Tool {
	var out []*Tool
	for i := range m.Tools {
		t := &m.Tools[i]
		if !t.Custom && (t.Fetch["github"] != nil || t.Fetch["pip"] != nil) {
			out = append(out, t)
		}
	}
	return out
}

func loadProvenance(base string) []Provenance { return (&App{Base: base}).loadProvenance() }

func (a *App) installedVersion2(t *Tool, prov []Provenance) string {
	if pk := pipPackages(t); len(pk) > 0 {
		return a.pipInstalled(pk[0])
	}
	for _, p := range prov {
		if p.Tool == t.ID {
			return p.Version
		}
	}
	return ""
}

type Updater struct{ infos map[string]UpdateInfo }

func newUpdater() *Updater { return &Updater{infos: map[string]UpdateInfo{}} }

// CheckUpdates controlla uno strumento alla volta (rispetta i limiti di GitHub) e notifica ogni risultato.
func (a *App) CheckUpdates(up *Updater, tools []*Tool, cb func(UpdateInfo)) {
	for _, t := range tools {
		inf := a.checkUpdate(t)
		if inf.Status == "non installato" {
			inf.Status = "da installare"
		}
		up.infos[t.ID] = inf
		cb(inf)
	}
}

func (a *App) UpdateTool(up *Updater, t *Tool, progress func(string)) (string, error) {
	inf, ok := up.infos[t.ID]
	if !ok {
		inf = a.checkUpdate(t)
	}
	if err := a.updateTool(t, inf, progress); err != nil {
		return "", err
	}
	if pk := pipPackages(t); len(pk) > 0 {
		return a.pipInstalled(pk[0]), nil
	}
	return inf.Latest, nil
}

var _ = time.Now

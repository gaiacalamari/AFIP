package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// --- release ufficiali ------------------------------------------------------------------------

type Asset struct {
	Name   string
	URL    string
	SHA256 string // digest pubblicato da GitHub, vuoto se la release non lo espone
	Size   int64
}

type Release struct {
	Repo      string // puo' differire dal catalogo se il progetto ha cambiato organizzazione
	Tag       string
	Published string
	Assets    []Asset
	Via       string // "api" | "pagina" | "pypi"
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

const userAgent = "AFIP-Portable/" + version

func httpGet(url string, accept string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return httpClient.Do(req)
}

// latestRelease legge l'ultima release dall'API di GitHub; se l'API non risponde (limite di 60
// richieste l'ora senza token) ricava le stesse informazioni dalla pagina pubblica della release.
func latestRelease(repo string) (*Release, error) {
	if r, err := latestViaAPI(repo); err == nil {
		return r, nil
	}
	return latestViaPage(repo)
}

func latestViaAPI(repo string) (*Release, error) {
	resp, err := httpGet("https://api.github.com/repos/"+repo+"/releases/latest", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API GitHub: %s", resp.Status)
	}
	var j struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	r := &Release{Repo: repo, Tag: j.TagName, Published: j.PublishedAt, Via: "api"}
	if m := regexp.MustCompile(`github\.com/([^/]+/[^/]+)/releases`).FindStringSubmatch(j.HTMLURL); m != nil {
		r.Repo = m[1]
	}
	for _, a := range j.Assets {
		r.Assets = append(r.Assets, Asset{Name: a.Name, URL: a.URL, SHA256: strings.TrimPrefix(a.Digest, "sha256:"), Size: a.Size})
	}
	return r, nil
}

var (
	reTagLoc   = regexp.MustCompile(`github\.com/([^/]+/[^/]+)/releases/tag/([^?#]+)`)
	reDigest   = regexp.MustCompile(`sha256:([0-9a-f]{64})`)
	reDateTime = regexp.MustCompile(`datetime="([0-9T:\-]+Z)"`)
)

func latestViaPage(repo string) (*Release, error) {
	noRedirect := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("HEAD", "https://github.com/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := noRedirect.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	// un repository rinominato fa prima un redirect al nuovo nome: lo si segue una volta
	if m := regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/releases/latest$`).FindStringSubmatch(loc); m != nil && m[1] != repo {
		return latestViaPage(m[1])
	}
	m := reTagLoc.FindStringSubmatch(loc)
	if m == nil {
		return nil, fmt.Errorf("nessuna release pubblicata per %s", repo)
	}
	r := &Release{Repo: m[1], Tag: m[2], Via: "pagina"}
	page, err := httpGet("https://github.com/"+r.Repo+"/releases/expanded_assets/"+r.Tag, "text/html")
	if err != nil {
		return nil, err
	}
	defer page.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(page.Body, 8<<20))
	html := string(b)
	// il nome del repository puo' differire nelle maiuscole (loki-rs / Loki-RS): si usa quello della pagina
	reAsset := regexp.MustCompile(`href="/((?i:` + regexp.QuoteMeta(r.Repo) + `))/releases/download/` + regexp.QuoteMeta(r.Tag) + `/([^"]+)"`)
	seen := map[string]bool{}
	for _, idx := range reAsset.FindAllStringSubmatchIndex(html, -1) {
		r.Repo = html[idx[2]:idx[3]]
		name := unescapeURL(html[idx[4]:idx[5]])
		if seen[name] {
			continue
		}
		seen[name] = true
		a := Asset{Name: name, URL: "https://github.com/" + r.Repo + "/releases/download/" + r.Tag + "/" + html[idx[4]:idx[5]]}
		end := idx[1] + 4000
		if end > len(html) {
			end = len(html)
		}
		if next := reAsset.FindStringIndex(html[idx[1]:end]); next != nil {
			end = idx[1] + next[0]
		}
		if d := reDigest.FindStringSubmatch(html[idx[1]:end]); d != nil {
			a.SHA256 = d[1]
		}
		r.Assets = append(r.Assets, a)
	}
	if d := reDateTime.FindStringSubmatch(html); d != nil {
		r.Published = d[1]
	}
	return r, nil
}

func unescapeURL(s string) string {
	return strings.NewReplacer("%2B", "+", "%20", " ", "%2b", "+").Replace(s)
}

func (r *Release) pick(patterns []string) ([]Asset, error) {
	var out []Asset
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		var hit *Asset
		for i := range r.Assets {
			if re.MatchString(r.Assets[i].Name) {
				hit = &r.Assets[i]
				break
			}
		}
		if hit == nil {
			return nil, fmt.Errorf("nessun file della release %s corrisponde a %s", r.Tag, p)
		}
		out = append(out, *hit)
	}
	return out, nil
}

// --- versioni installate (manifest/provenance.json) --------------------------------------------

type Provenance struct {
	Tool         string `json:"tool"`
	Source       string `json:"source"`
	Version      string `json:"version"`
	File         string `json:"file"`
	URL          string `json:"url"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	Declared     string `json:"github_declared_sha256,omitempty"`
	Verified     bool   `json:"verified"`
	DownloadedAt string `json:"downloaded_at"`
	Published    string `json:"published_at,omitempty"`
}

func (a *App) provPath() string { return filepath.Join(a.Base, "manifest", "provenance.json") }

func (a *App) loadProvenance() []Provenance {
	var p []Provenance
	if b, err := os.ReadFile(a.provPath()); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

func (a *App) installedVersion(id string) string {
	for _, p := range a.loadProvenance() {
		if p.Tool == id {
			return p.Version
		}
	}
	return ""
}

// --- controllo e aggiornamento ----------------------------------------------------------------

type UpdateInfo struct {
	ID, Name   string
	Installed  string
	Latest     string
	Published  string
	Status     string // aggiornato | disponibile | non installato | errore
	Err        string
	release    *Release
	pipVersion string
}

func pipPackages(t *Tool) []string {
	var out []string
	if v, ok := t.Fetch["pip"].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func strList(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			s, _ := x.(string)
			out = append(out, s)
		}
	}
	return out
}

func (a *App) checkUpdate(t *Tool) UpdateInfo {
	u := UpdateInfo{ID: t.ID, Name: t.Name}
	if pk := pipPackages(t); len(pk) > 0 {
		u.Installed = a.pipInstalled(pk[0])
		v, err := pypiLatest(pk[0])
		if err != nil {
			u.Status, u.Err = "errore", err.Error()
			return u
		}
		u.Latest, u.pipVersion = v, v
	} else {
		repo, _ := t.Fetch["github"].(string)
		if repo == "" {
			u.Status = "manuale"
			return u
		}
		u.Installed = a.installedVersion(t.ID)
		r, err := latestRelease(repo)
		if err != nil {
			u.Status, u.Err = "errore", err.Error()
			return u
		}
		if _, err := r.pick(strList(t.Fetch["assets"])); err != nil {
			u.Status, u.Err = "errore", err.Error()
			return u
		}
		u.Latest, u.Published, u.release = r.Tag, r.Published, r
	}
	switch {
	case u.Installed == "":
		u.Status = "non installato"
	case u.Installed == u.Latest:
		u.Status = "aggiornato"
	default:
		u.Status = "disponibile"
	}
	return u
}

func pypiLatest(pkg string) (string, error) {
	resp, err := httpGet("https://pypi.org/pypi/"+pkg+"/json", "application/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("PyPI: %s", resp.Status)
	}
	var j struct {
		Info struct{ Version string } `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return "", err
	}
	return j.Info.Version, nil
}

func (a *App) pipInstalled(pkg string) string {
	sp := a.sitePackages()
	if sp == "" {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(sp, strings.ReplaceAll(pkg, "-", "_")+"-*.dist-info"))
	if len(m) == 0 {
		m, _ = filepath.Glob(filepath.Join(sp, pkg+"-*.dist-info"))
	}
	if len(m) == 0 {
		return ""
	}
	n := strings.TrimSuffix(filepath.Base(m[0]), ".dist-info")
	return n[strings.LastIndex(n, "-")+1:]
}

var safeName = regexp.MustCompile(`[^\w.\-]`)

// updateTool scarica, verifica e installa l'ultima release di uno strumento.
// La nuova versione viene preparata a parte e sostituisce la vecchia solo se tutto e' andato bene.
func (a *App) updateTool(t *Tool, info UpdateInfo, progress func(string)) error {
	if pk := pipPackages(t); len(pk) > 0 {
		return a.updatePip(t, pk, info, progress)
	}
	r := info.release
	if r == nil {
		return errors.New("controllare prima gli aggiornamenti")
	}
	assets, err := r.pick(strList(t.Fetch["assets"]))
	if err != nil {
		return err
	}
	filters := strList(t.Fetch["filters"])
	unpack, _ := t.Fetch["unpack"].(string)
	toolsDir := filepath.Join(a.Base, "tools")
	staging := filepath.Join(toolsDir, ".nuovo-"+t.ID)
	os.RemoveAll(staging)
	dest := filepath.Join(staging, safeName.ReplaceAllString(r.Tag, "_"))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	var provs []Provenance
	for i, as := range assets {
		progress(fmt.Sprintf("scarico %s", as.Name))
		file := filepath.Join(staging, ".scaricati", as.Name)
		sum, size, err := download(as.URL, file)
		if err != nil {
			os.RemoveAll(staging)
			return err
		}
		if as.SHA256 != "" && !strings.EqualFold(as.SHA256, sum) {
			os.Remove(file)
			os.RemoveAll(staging)
			return fmt.Errorf("%s: SHA-256 diverso da quello pubblicato su GitHub, file scartato", as.Name)
		}
		progress(fmt.Sprintf("estraggo %s", as.Name))
		filter := ""
		if i < len(filters) {
			filter = filters[i]
		}
		mode := unpack
		switch low := strings.ToLower(as.Name); {
		case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
			mode = "tar.gz"
		case strings.HasSuffix(low, ".gz"):
			mode = "gz"
		case !strings.HasSuffix(low, ".zip"):
			mode = "copy"
		}
		err = extract(file, dest, mode, filter)
		os.Remove(file) // l'archivio non resta nella pendrive: il suo SHA-256 e' nella provenienza
		if err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("%s: %w", as.Name, err)
		}
		provs = append(provs, Provenance{Tool: t.ID, Source: "github:" + r.Repo, Version: r.Tag, File: as.Name, URL: as.URL, Size: size,
			SHA256: sum, Declared: as.SHA256, Verified: as.SHA256 != "", DownloadedAt: time.Now().UTC().Format(time.RFC3339), Published: r.Published})
	}
	os.RemoveAll(filepath.Join(staging, ".scaricati"))
	// l'eseguibile atteso deve esserci, altrimenti non si tocca la versione installata
	probe := *t
	probe.ID, probe.SearchDirs = ".nuovo-"+t.ID, nil
	if len(t.Exe) > 0 && findExe(a.Base, &probe, t.Exe) == "" {
		os.RemoveAll(staging)
		return fmt.Errorf("il pacchetto non contiene %s: aggiornamento annullato", strings.Join(t.Exe, " o "))
	}
	var pipBefore []string
	if t.ID == "python" {
		pipBefore = a.installedPipPackages()
	}
	progress("sostituisco la versione installata")
	final := filepath.Join(toolsDir, t.ID)
	old := filepath.Join(toolsDir, ".vecchio-"+t.ID)
	os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			os.RemoveAll(staging)
			return fmt.Errorf("impossibile sostituire %s (lo strumento e' aperto?): %v", t.Name, err)
		}
	}
	if err := os.Rename(staging, final); err != nil {
		os.Rename(old, final)
		return err
	}
	os.RemoveAll(old)
	// provenienza e sigillo aggiornati solo per questo strumento
	all := a.loadProvenance()
	keep := all[:0]
	for _, p := range all {
		if p.Tool != t.ID {
			keep = append(keep, p)
		}
	}
	keep = append(keep, provs...)
	b, _ := json.MarshalIndent(keep, "", "  ")
	os.WriteFile(a.provPath(), b, 0o644)
	if err := a.resealPrefix("tools/" + t.ID + "/"); err != nil {
		return err
	}
	if t.ID == "python" && len(pipBefore) > 0 && runtime.GOOS == "windows" {
		// il nuovo Python parte vuoto: si reinstallano Volatility 3, MVT ecc. che erano presenti
		progress("reinstallo " + strings.Join(pipBefore, ", "))
		cmd := exec.Command(a.pythonExe(), append([]string{"-m", "pip", "install", "--no-warn-script-location", "--disable-pip-version-check"}, pipBefore...)...)
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			return fmt.Errorf("Python aggiornato, ma la reinstallazione di %s non è riuscita: %s", strings.Join(pipBefore, ", "), lines[len(lines)-1])
		}
		a.resealPrefix(a.pythonPrefix())
	}
	verified := "hash registrato"
	if len(provs) > 0 && provs[0].Verified {
		verified = "SHA-256 verificato con GitHub"
	}
	a.log(LogEntry{Action: "tool.update", Tool: t.ID, SHA256: provs[0].SHA256,
		Detail: fmt.Sprintf("%s -> %s da %s (%s)", orNone(info.Installed), r.Tag, r.Repo, verified)})
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "nessuna"
	}
	return s
}

func (a *App) updatePip(t *Tool, pkgs []string, info UpdateInfo, progress func(string)) error {
	py := a.pythonExe()
	if py == "" {
		return errors.New("manca il Python portatile: installarlo prima da questa stessa schermata")
	}
	progress("pip install --upgrade " + strings.Join(pkgs, " "))
	cmd := exec.Command(py, append([]string{"-m", "pip", "install", "--upgrade", "--no-warn-script-location", "--disable-pip-version-check"}, pkgs...)...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("pip: %s", lines[len(lines)-1])
	}
	if err := a.resealPrefix(a.pythonPrefix()); err != nil {
		return err
	}
	a.log(LogEntry{Action: "tool.update", Tool: t.ID, Detail: fmt.Sprintf("%s -> %s da PyPI", orNone(info.Installed), a.pipInstalled(pkgs[0]))})
	return nil
}

// download scrive il file in una copia temporanea calcolando l'SHA-256 durante la ricezione.
func download(url, dest string) (string, int64, error) {
	os.MkdirAll(filepath.Dir(dest), 0o755)
	resp, err := httpGet(url, "")
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("download %s: %s", filepath.Base(dest), resp.Status)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	os.Remove(dest)
	return hex.EncodeToString(h.Sum(nil)), n, os.Rename(tmp, dest)
}

// extract gestisce zip (anche con percorsi Windows "a\b"), gz e copia semplice.
// filter, se presente, estrae solo i percorsi che corrispondono. Rifiuta percorsi che escono da dest.
func extract(file, dest, mode, filter string) error {
	switch mode {
	case "gz":
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		defer in.Close()
		gz, err := gzip.NewReader(in)
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(dest, strings.TrimSuffix(filepath.Base(file), ".gz")))
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, gz)
		return err
	case "tar.gz":
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		defer in.Close()
		gz, err := gzip.NewReader(in)
		if err != nil {
			return err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			parts, err := safeParts(h.Name)
			if err != nil {
				return err
			}
			if len(parts) == 0 {
				continue
			}
			target := filepath.Join(append([]string{dest}, parts...)...)
			switch h.Typeflag {
			case tar.TypeDir:
				os.MkdirAll(target, 0o755)
			case tar.TypeReg:
				os.MkdirAll(filepath.Dir(target), 0o755)
				out, err := os.Create(target)
				if err != nil {
					return err
				}
				_, err = io.Copy(out, tr)
				out.Close()
				if err != nil {
					return err
				}
			} // collegamenti simbolici e speciali: ignorati (non servono e sarebbero un rischio)
		}
	case "zip":
		var re *regexp.Regexp
		if filter != "" {
			re = regexp.MustCompile(filter)
		}
		z, err := zip.OpenReader(file)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			name := strings.ReplaceAll(f.Name, "\\", "/")
			if re != nil && !re.MatchString(name) {
				continue
			}
			var parts []string
			for _, p := range strings.Split(name, "/") {
				if p == "" || p == "." {
					continue
				}
				if p == ".." {
					return fmt.Errorf("percorso non sicuro nello zip: %s", f.Name)
				}
				parts = append(parts, p)
			}
			if len(parts) == 0 {
				continue
			}
			target := filepath.Join(append([]string{dest}, parts...)...)
			if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
				os.MkdirAll(target, 0o755)
				continue
			}
			os.MkdirAll(filepath.Dir(target), 0o755)
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	default:
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(filepath.Join(dest, filepath.Base(file)))
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	}
}

// resealPrefix ricalcola il sigillo solo per una parte del toolkit (uno strumento aggiornato o aggiunto).
func (a *App) resealPrefix(prefix string) error {
	s := loadSeal(a.Base)
	if s == nil {
		_, err := createSeal(a.Base, a.Host, a.User)
		return err
	}
	for k := range s.Files {
		if strings.HasPrefix(k, prefix) {
			delete(s.Files, k)
		}
	}
	root := filepath.Join(a.Base, filepath.FromSlash(strings.TrimSuffix(prefix, "/")))
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		sum, size, err := hashFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(a.Base, p)
		s.Files[filepath.ToSlash(rel)] = SealFile{sum, size}
		return nil
	})
	if err != nil {
		return err
	}
	s.Count, s.Digest = len(s.Files), digestOf(s.Files)
	s.Created, s.Host, s.User = time.Now().UTC().Format(time.RFC3339), a.Host, a.User
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(sealPath(a.Base), b, 0o644)
}

func sortedIDs(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func safeParts(name string) ([]string, error) {
	var parts []string
	for _, p := range strings.Split(strings.ReplaceAll(name, "\\", "/"), "/") {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return nil, fmt.Errorf("percorso non sicuro nell'archivio: %s", name)
		}
		parts = append(parts, p)
	}
	return parts, nil
}

// pythonExe trova il Python portatile: runtime/python (pacchetto completo) oppure tools/python/<versione>/python
// (installato dagli aggiornamenti).
func (a *App) pythonExe() string {
	exe := "python.exe"
	if runtime.GOOS != "windows" {
		exe = "python3"
	}
	if p := filepath.Join(a.Base, "runtime", "python", exe); fileExists(p) {
		return p
	}
	m, _ := filepath.Glob(filepath.Join(a.Base, "tools", "python", "*", "python", exe))
	sort.Strings(m)
	if len(m) > 0 {
		return m[len(m)-1]
	}
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// sitePackages del Python portatile in uso (vuoto se non c'e').
func (a *App) sitePackages() string {
	py := a.pythonExe()
	if py == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(py), "Lib", "site-packages")
}

// prefisso del sigillo che contiene il Python in uso
func (a *App) pythonPrefix() string {
	py := a.pythonExe()
	if py == "" {
		return "runtime/"
	}
	rel, _ := filepath.Rel(a.Base, py)
	if strings.HasPrefix(filepath.ToSlash(rel), "runtime/") {
		return "runtime/"
	}
	return "tools/python/"
}

// installedPipPackages: pacchetti del catalogo (Volatility 3, MVT…) presenti nel Python in uso.
func (a *App) installedPipPackages() []string {
	m, err := loadManifest(a.Base)
	if err != nil {
		return nil
	}
	var out []string
	for i := range m.Tools {
		for _, pk := range pipPackages(&m.Tools[i]) {
			if a.pipInstalled(pk) != "" {
				out = append(out, pk)
			}
		}
	}
	return out
}

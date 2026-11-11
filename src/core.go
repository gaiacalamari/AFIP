package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const version = "1.3.0"

// Launcher astrae l'avvio dei processi, cosi' i test non lanciano programmi reali.
type Launcher interface {
	GUI(exe string, args []string, dir string, env []string) error
	Console(title, hint, dir string, pathDirs []string, env []string, scratch string) error
	OpenURL(url string) error
	OpenFolder(path string) error
}

type App struct {
	Base     string
	Host     string
	User     string
	SelfHash string
	Ledger   *Ledger
	cache    *hashCache
	launcher Launcher
}

func newApp(base string, l Launcher) *App {
	host, _ := os.Hostname()
	uname := os.Getenv("USERNAME")
	if u, err := user.Current(); err == nil {
		uname = u.Username
	}
	a := &App{Base: base, Host: host, User: uname, launcher: l,
		Ledger: &Ledger{path: filepath.Join(base, "logs", "launch-log.jsonl")},
		cache:  &hashCache{m: map[string]cacheEntry{}}}
	if exe, err := os.Executable(); err == nil {
		a.SelfHash, _, _ = hashFile(exe)
	}
	return a
}

func (a *App) log(e LogEntry) (LogEntry, error) {
	e.Host, e.User = a.Host, a.User
	return a.Ledger.Append(e)
}

// --- stato degli strumenti --------------------------------------------------------------------
type ToolState struct {
	Tool
	Status     string // pronto | non_sigillato | modificato | presente | mancante | non_disponibile | url
	ExePath    string
	ConsoleOK  bool
	SHA256     string
	SealedHash string
	ToolDir    string
}

func (a *App) rel(p string) string {
	r, err := filepath.Rel(a.Base, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

// toolState con hash=false fa solo la ricerca del file (veloce, per il primo disegno del menu).
func (a *App) toolState(t *Tool, seal *Seal, hash bool) ToolState {
	st := ToolState{Tool: *t, ToolDir: filepath.Join(a.Base, "tools", t.ID)}
	if !runnable(t) {
		st.Status = "non_disponibile"
		return st
	}
	if t.Availability == "server" {
		st.Status = "url"
		return st
	}
	if pk := pipPackages(t); len(pk) > 0 && a.pipInstalled(pk[0]) == "" {
		st.Status = "mancante" // senza Python portatile o senza il pacchetto, il comando .cmd non funzionerebbe
		return st
	}
	exe := findExe(a.Base, t, t.Exe)
	if exe == "" && len(t.ConsoleExe) > 0 {
		exe = findExe(a.Base, t, t.ConsoleExe)
	}
	if exe == "" {
		st.Status = "mancante"
		return st
	}
	st.ExePath = a.rel(exe)
	st.ConsoleOK = t.Kind == "cli" || len(t.ConsoleExe) > 0
	if !hash {
		st.Status = "presente"
		return st
	}
	sum, err := a.cache.get(exe)
	if err != nil {
		st.Status = "mancante"
		return st
	}
	st.SHA256 = sum
	switch {
	case seal == nil || seal.Files[st.ExePath].SHA256 == "":
		st.Status = "non_sigillato"
	case seal.Files[st.ExePath].SHA256 == sum:
		st.Status, st.SealedHash = "pronto", sum
	default:
		st.Status, st.SealedHash = "modificato", seal.Files[st.ExePath].SHA256
	}
	return st
}

func (a *App) states(m *Manifest, hash bool) []ToolState {
	seal := loadSeal(a.Base)
	out := make([]ToolState, 0, len(m.Tools))
	for i := range m.Tools {
		out = append(out, a.toolState(&m.Tools[i], seal, hash))
	}
	return out
}

// --- avvio ------------------------------------------------------------------------------------
type LaunchReq struct {
	ID        string
	Mode      string // gui | console | url
	Confirmed bool   // conferma dell'operatore per gli strumenti che accedono a credenziali
	Force     bool
	Reason    string
}

// Errori che l'interfaccia trasforma in una domanda all'operatore.
var (
	ErrNeedConfirm = errors.New("questo strumento accede a credenziali: serve la conferma dell'operatore")
	ErrNeedReason  = errors.New("l'eseguibile non corrisponde al sigillo del toolkit: avvio bloccato")
)

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func expand(s, toolDir, outDir string) string {
	return strings.NewReplacer("%AFIP_TOOL%", toolDir, "%AFIP_OUTPUT%", outDir).Replace(s)
}

// outputDir: cartella di lavoro delle console, fuori da tools/ e runtime/ (quindi fuori dal sigillo).
func (a *App) outputDir() string {
	d := filepath.Join(a.Base, "output")
	os.MkdirAll(d, 0o755)
	return d
}

func (a *App) launch(r LaunchReq) (LogEntry, error) {
	m, err := loadManifest(a.Base)
	if err != nil {
		return LogEntry{}, err
	}
	t := m.find(r.ID)
	if t == nil {
		return LogEntry{}, errors.New("strumento sconosciuto")
	}
	if t.AuthRequired && !r.Confirmed {
		return LogEntry{}, ErrNeedConfirm
	}
	entry := LogEntry{Tool: t.ID}
	if t.AuthRequired {
		entry.AuthRef = "uso autorizzato confermato dall'operatore"
	}
	if t.Availability == "server" {
		if t.URL == "" {
			return entry, errors.New("nessun indirizzo configurato in tools.json")
		}
		entry.Action, entry.Detail = "open.url", t.URL
		logged, err := a.log(entry)
		if err != nil {
			return logged, err
		}
		return logged, a.failed(entry, a.launcher.OpenURL(t.URL))
	}
	st := a.toolState(t, loadSeal(a.Base), true)
	switch st.Status {
	case "non_disponibile":
		return entry, errors.New("non eseguibile su Windows. " + t.Alternative)
	case "mancante":
		return entry, fmt.Errorf("file non trovato: copiarlo in %s", st.ToolDir)
	case "modificato":
		if !r.Force || len(strings.TrimSpace(r.Reason)) < 4 {
			return entry, ErrNeedReason
		}
		entry.Reason = "avvio forzato su hash non corrispondente: " + strings.TrimSpace(r.Reason)
	}
	exe := filepath.Join(a.Base, filepath.FromSlash(st.ExePath))
	toolDir := filepath.Join(a.Base, "tools", t.ID)
	outDir := a.outputDir()
	// PYTHONDONTWRITEBYTECODE: Python non scrive cache nella pendrive, il sigillo resta pulito.
	env := append(os.Environ(), "AFIP_TOOL="+toolDir, "AFIP_OUTPUT="+outDir, "AFIP_BASE="+a.Base, "PYTHONDONTWRITEBYTECODE=1")
	if py := a.pythonExe(); py != "" {
		env = append(env, "AFIP_PYTHON="+py) // i comandi vol, mvt… usano direttamente questo Python
	}
	entry.Exe, entry.SHA256 = st.ExePath, st.SHA256
	if r.Mode == "console" || (r.Mode == "" && t.Kind == "cli") {
		dirs := []string{filepath.Dir(exe)}
		if c := findExe(a.Base, t, t.ConsoleExe); c != "" {
			dirs = append([]string{filepath.Dir(c)}, dirs...)
		}
		dirs = append(dirs, filepath.Join(a.Base, "runtime", "bin"))
		entry.Action = "launch.console"
		logged, err := a.log(entry) // si registra PRIMA di avviare
		if err != nil {
			return logged, err
		}
		return logged, a.failed(entry, a.launcher.Console(t.Name, t.Hint, outDir, dirs, env, filepath.Join(a.Base, "logs")))
	}
	args := make([]string, 0, len(t.Args))
	for _, s := range t.Args {
		args = append(args, expand(s, toolDir, outDir))
	}
	entry.Action = "launch.gui"
	logged, err := a.log(entry)
	if err != nil {
		return entry, err
	}
	return logged, a.failed(entry, a.launcher.GUI(exe, args, filepath.Dir(exe), env))
}

// failed annota nel registro un avvio non riuscito, cosi' la voce di avvio non resta senza esito.
func (a *App) failed(e LogEntry, err error) error {
	if err != nil {
		e.Action, e.Detail, e.Reason = "launch.failed", err.Error(), ""
		a.log(e)
	}
	return err
}

func (a *App) seal() (*Seal, error) {
	s, err := createSeal(a.Base, a.Host, a.User)
	if err == nil {
		a.log(LogEntry{Action: "toolkit.seal", Detail: fmt.Sprintf("%d file, digest %s", s.Count, s.Digest)})
	}
	return s, err
}

func (a *App) verify() (*ToolkitDiff, error) {
	d, err := verifyToolkit(a.Base)
	if err == nil && d.Sealed {
		res := "integro"
		if !d.Valid {
			res = fmt.Sprintf("DIFFERENZE: %d modificati, %d mancanti, %d aggiunti", len(d.Changed), len(d.Missing), len(d.Added))
		}
		a.log(LogEntry{Action: "toolkit.verify", Detail: res})
	}
	return d, err
}

// --- impostazioni della finestra (salvate sulla pendrive) --------------------------------------
type Settings struct {
	X, Y, W, H      int
	Topmost         bool
	Compact         bool
	ShowUnavailable bool
	OnlyPresent     bool
	Collapsed       map[string]bool
}

func (a *App) settingsPath() string { return filepath.Join(a.Base, "manifest", "settings.json") }

func (a *App) loadSettings() Settings {
	s := Settings{Topmost: true, OnlyPresent: true, Collapsed: map[string]bool{}}
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.Collapsed == nil {
		s.Collapsed = map[string]bool{}
	}
	return s
}

func (a *App) saveSettings(s Settings) {
	b, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(a.settingsPath(), b, 0o644)
}

// cmdEscape protegge i caratteri speciali di cmd.exe nelle righe di echo.
func cmdEscape(s string) string {
	r := strings.NewReplacer("^", "^^", "&", "^&", "|", "^|", "<", "^<", ">", "^>", "(", "^(", ")", "^)", "%", "%%")
	return r.Replace(s)
}

func baseDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return filepath.Dir(exe)
}

func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func shortHash(s string) string {
	h, _, _ := hashReader(s)
	return h[:12]
}

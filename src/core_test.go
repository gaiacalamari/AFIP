package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeLauncher struct{ calls []string }

func (f *fakeLauncher) GUI(exe string, args []string, dir string, env []string) error {
	f.calls = append(f.calls, "gui:"+filepath.Base(exe)+":"+strings.Join(args, ","))
	return nil
}
func (f *fakeLauncher) Console(title, hint, dir string, pathDirs []string, env []string, scratch string) error {
	f.calls = append(f.calls, "console:"+title+":"+dir)
	return nil
}
func (f *fakeLauncher) OpenURL(u string) error    { f.calls = append(f.calls, "url:"+u); return nil }
func (f *fakeLauncher) OpenFolder(p string) error { f.calls = append(f.calls, "folder:"+p); return nil }

type failingLauncher struct{ fakeLauncher }

func (f *failingLauncher) Console(title, hint, dir string, pathDirs []string, env []string, scratch string) error {
	return os.ErrPermission
}

const testManifest = `{"schema":1,"categories":["A"],"tools":[
 {"id":"guitool","name":"Gui Tool","category":"A","kind":"gui","availability":"portable","exe":["gui*.exe"],"args":["-datadir=%AFIP_TOOL%\\data"]},
 {"id":"clitool","name":"Cli Tool","category":"A","kind":"cli","availability":"portable","exe":["cli.exe"],"hint":"cli -x | more"},
 {"id":"pw","name":"Password Tool","category":"A","kind":"cli","availability":"portable","exe":["pw.exe"],"auth_required":true},
 {"id":"srv","name":"Server","category":"A","kind":"gui","availability":"server","url":"http://localhost:5601"},
 {"id":"lin","name":"Linux only","category":"A","kind":"cli","availability":"linux","alternative":"usa Tsurugi"},
 {"id":"py","name":"Py","category":"A","kind":"cli","availability":"python","exe":["vol.cmd"],"search_dirs":["runtime/bin"]},
 {"id":"missing","name":"Missing","category":"A","kind":"gui","availability":"portable","exe":["nope.exe"]}
]}`

func setup(t *testing.T, l Launcher) (*App, string) {
	t.Helper()
	base := t.TempDir()
	for _, d := range []string{"manifest", "tools/guitool/v1.2/bin", "tools/clitool", "tools/pw", "runtime/bin", "logs"} {
		os.MkdirAll(filepath.Join(base, d), 0o755)
	}
	os.WriteFile(filepath.Join(base, "manifest", "tools.json"), []byte(testManifest), 0o644)
	os.WriteFile(filepath.Join(base, "tools/guitool/v1.2/bin/GuiApp.exe"), []byte("gui-binary"), 0o755)
	os.WriteFile(filepath.Join(base, "tools/clitool/cli.exe"), []byte("cli-binary"), 0o755)
	os.WriteFile(filepath.Join(base, "tools/pw/pw.exe"), []byte("pw-binary"), 0o755)
	os.WriteFile(filepath.Join(base, "runtime/bin/vol.cmd"), []byte("@python -c ..."), 0o644)
	return newApp(base, l), base
}

func statusMap(a *App) map[string]string {
	m, _ := loadManifest(a.Base)
	out := map[string]string{}
	for _, s := range a.states(m, true) {
		out[s.ID] = s.Status
	}
	return out
}

func TestStatuses(t *testing.T) {
	a, _ := setup(t, &fakeLauncher{})
	want := map[string]string{"guitool": "non_sigillato", "clitool": "non_sigillato", "srv": "url", "lin": "non_disponibile", "py": "non_sigillato", "missing": "mancante"}
	got := statusMap(a)
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: atteso %s, ottenuto %s", id, w, got[id])
		}
	}
	m, _ := loadManifest(a.Base)
	if q := a.states(m, false); q[0].Status != "presente" {
		t.Errorf("il primo passaggio senza hash deve dare 'presente', non %s", q[0].Status)
	}
}

func TestSealThenTamperBlocksLaunch(t *testing.T) {
	fl := &fakeLauncher{}
	a, base := setup(t, fl)
	if _, err := a.seal(); err != nil {
		t.Fatal(err)
	}
	if statusMap(a)["clitool"] != "pronto" {
		t.Fatal("dopo il sigillo il tool deve essere pronto")
	}
	os.WriteFile(filepath.Join(base, "tools/clitool/cli.exe"), []byte("cli-binary-trojan"), 0o755)
	if statusMap(a)["clitool"] != "modificato" {
		t.Fatal("la modifica dell'eseguibile deve essere rilevata")
	}
	if _, err := a.launch(LaunchReq{ID: "clitool"}); err != ErrNeedReason || len(fl.calls) != 0 {
		t.Fatalf("avvio di un eseguibile modificato non bloccato: %v %v", err, fl.calls)
	}
	if _, err := a.launch(LaunchReq{ID: "clitool", Force: true, Reason: "aggiornato, da risigillare"}); err != nil || len(fl.calls) != 1 {
		t.Fatal("avvio forzato con motivazione deve partire")
	}
	all, _ := a.Ledger.readAll()
	if !strings.Contains(all[len(all)-1].Reason, "forzato") {
		t.Fatalf("la motivazione deve essere nel registro: %+v", all[len(all)-1])
	}
	d, _ := a.verify()
	if d.Valid || len(d.Changed) != 1 {
		t.Fatalf("verifica toolkit: %+v", d)
	}
}

func TestCredentialToolNeedsConfirmation(t *testing.T) {
	fl := &fakeLauncher{}
	a, base := setup(t, fl)
	if _, err := a.launch(LaunchReq{ID: "pw"}); err != ErrNeedConfirm || len(fl.calls) != 0 {
		t.Fatalf("serve la conferma: %v %v", err, fl.calls)
	}
	e, err := a.launch(LaunchReq{ID: "pw", Confirmed: true})
	if err != nil || e.AuthRef == "" || len(fl.calls) != 1 || !strings.Contains(fl.calls[0], filepath.Join(base, "output")) {
		t.Fatalf("avvio confermato: %v %+v %v", err, e, fl.calls)
	}
	if _, err := os.Stat(filepath.Join(base, "cases")); err == nil {
		t.Fatal("non deve essere creata nessuna cartella dei casi")
	}
}

func TestDefaultModeAndArgs(t *testing.T) {
	fl := &fakeLauncher{}
	a, base := setup(t, fl)
	a.launch(LaunchReq{ID: "guitool"})
	a.launch(LaunchReq{ID: "clitool"})
	a.launch(LaunchReq{ID: "srv"})
	if len(fl.calls) != 3 || !strings.HasPrefix(fl.calls[0], "gui:GuiApp.exe:-datadir="+filepath.Join(base, "tools", "guitool")) ||
		!strings.HasPrefix(fl.calls[1], "console:Cli Tool") || fl.calls[2] != "url:http://localhost:5601" {
		t.Fatalf("modalita' predefinite: %v", fl.calls)
	}
	if _, err := a.launch(LaunchReq{ID: "lin"}); err == nil || !strings.Contains(err.Error(), "Tsurugi") {
		t.Fatalf("tool solo Linux: %v", err)
	}
	if _, err := a.launch(LaunchReq{ID: "missing"}); err == nil || !strings.Contains(err.Error(), "non trovato") {
		t.Fatalf("tool mancante: %v", err)
	}
}

func TestFailedLaunchIsLogged(t *testing.T) {
	a, _ := setup(t, &failingLauncher{})
	if _, err := a.launch(LaunchReq{ID: "clitool"}); err == nil {
		t.Fatal("atteso errore")
	}
	all, _ := a.Ledger.readAll()
	if len(all) != 2 || all[0].Action != "launch.console" || all[1].Action != "launch.failed" {
		t.Fatalf("registro: %+v", all)
	}
}

func TestLedgerChainDetectsTampering(t *testing.T) {
	a, base := setup(t, &fakeLauncher{})
	for i := 0; i < 3; i++ {
		a.log(LogEntry{Action: "test"})
	}
	if c := a.Ledger.Verify(); !c.Valid || c.Entries != 3 {
		t.Fatalf("catena valida attesa: %+v", c)
	}
	p := filepath.Join(base, "logs", "launch-log.jsonl")
	b, _ := os.ReadFile(p)
	user, _ := json.Marshal(a.User)
	os.WriteFile(p, bytes.Replace(b, []byte(`"user":`+string(user)), []byte(`"user":"x"`), 1), 0o644)
	if c := a.Ledger.Verify(); c.Valid || c.BrokenAt != 1 {
		t.Fatalf("manomissione non rilevata: %+v", c)
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	os.WriteFile(p, append(bytes.Join(append(lines[:1:1], lines[2:]...), []byte("\n")), '\n'), 0o644)
	if c := a.Ledger.Verify(); c.Valid || c.BrokenAt != 2 {
		t.Fatalf("cancellazione non rilevata: %+v", c)
	}
}

func TestSettings(t *testing.T) {
	a, _ := setup(t, &fakeLauncher{})
	s := a.loadSettings()
	if !s.Topmost {
		t.Fatal("predefinito: sempre in primo piano")
	}
	s.Collapsed["Rete"], s.X, s.OnlyPresent = true, 120, true
	a.saveSettings(s)
	if r := a.loadSettings(); !r.Collapsed["Rete"] || r.X != 120 || !r.OnlyPresent {
		t.Fatalf("impostazioni non salvate: %+v", r)
	}
}

func TestConsoleEnvKeepsSealClean(t *testing.T) {
	a, _ := setup(t, &fakeLauncher{})
	a.seal()
	a.launch(LaunchReq{ID: "clitool"})
	d, _ := a.verify()
	if !d.Valid {
		t.Fatalf("l'avvio non deve sporcare il toolkit sigillato: %+v", d)
	}
}

func TestCmdEscape(t *testing.T) {
	if got := cmdEscape(`a | b & c > d (e) 50%`); got != `a ^| b ^& c ^> d ^(e^) 50%%` {
		t.Fatal(got)
	}
}

func TestRealManifest(t *testing.T) {
	b, err := os.ReadFile("../manifest/tools.json")
	if err != nil {
		t.Skip("manifest reale non trovato")
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	seen, cats := map[string]bool{}, map[string]bool{}
	for _, c := range m.Categories {
		cats[c] = true
	}
	for _, x := range m.Tools {
		if seen[x.ID] || slugRe.MatchString(x.ID) {
			t.Errorf("id duplicato o non valido: %s", x.ID)
		}
		seen[x.ID] = true
		if !cats[x.Category] {
			t.Errorf("%s: categoria sconosciuta %s", x.ID, x.Category)
		}
		if runnable(&x) && x.Availability != "server" && len(x.Exe) == 0 && len(x.ConsoleExe) == 0 {
			t.Errorf("%s: nessun eseguibile indicato", x.ID)
		}
	}
}

func TestLaunchReturnsLoggedSeq(t *testing.T) {
	a, _ := setup(t, &fakeLauncher{})
	a.log(LogEntry{Action: "session.start"})
	e, err := a.launch(LaunchReq{ID: "clitool"})
	if err != nil || e.Seq != 2 || e.Hash == "" {
		t.Fatalf("la voce restituita deve essere quella registrata: %+v %v", e, err)
	}
}

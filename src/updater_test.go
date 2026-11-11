package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, _ := w.Create(name)
		f.Write([]byte(body))
	}
	w.Close()
	os.WriteFile(path, buf.Bytes(), 0o644)
}

func TestExtractZipBackslashFilterAndSlip(t *testing.T) {
	dir := t.TempDir()
	z := filepath.Join(dir, "a.zip")
	writeZip(t, z, map[string]string{`plugins\m_vmemd.dll`: "x", "chainsaw/rules/r.yml": "r", "chainsaw/chainsaw_linux": "bin", "top.exe": "e"})
	out := filepath.Join(dir, "out")
	os.MkdirAll(out, 0o755)
	if err := extract(z, out, "zip", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "plugins", "m_vmemd.dll")); err != nil {
		t.Fatal("i percorsi con barra rovesciata devono diventare cartelle")
	}
	out2 := filepath.Join(dir, "out2")
	os.MkdirAll(out2, 0o755)
	extract(z, out2, "zip", "^chainsaw/(rules|sigma)/")
	if _, err := os.Stat(filepath.Join(out2, "chainsaw", "rules", "r.yml")); err != nil {
		t.Fatal("il filtro deve estrarre le regole")
	}
	if _, err := os.Stat(filepath.Join(out2, "chainsaw", "chainsaw_linux")); err == nil {
		t.Fatal("il filtro deve escludere il resto")
	}
	bad := filepath.Join(dir, "bad.zip")
	writeZip(t, bad, map[string]string{"../../evil.txt": "x"})
	if err := extract(bad, out, "zip", ""); err == nil || !strings.Contains(err.Error(), "non sicuro") {
		t.Fatalf("zip-slip non bloccato: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.txt")); err == nil {
		t.Fatal("file scritto fuori dalla cartella")
	}
}

func TestPickAssets(t *testing.T) {
	r := &Release{Tag: "v4.4.0", Assets: []Asset{{Name: "hayabusa-4.4.0-win-x64-live-response.zip"}, {Name: "hayabusa-4.4.0-win-x64.zip"}}}
	got, err := r.pick([]string{`^hayabusa-[0-9.]+-win-x64\.zip$`})
	if err != nil || got[0].Name != "hayabusa-4.4.0-win-x64.zip" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := r.pick([]string{`^nope$`}); err == nil {
		t.Fatal("pattern senza corrispondenza deve dare errore")
	}
}

func TestCustomToolsAddRemoveAndSeal(t *testing.T) {
	a, base := setup(t, &fakeLauncher{})
	a.seal()
	src := filepath.Join(t.TempDir(), "MyTool")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "mytool.exe"), []byte("MZ-not-really"), 0o755)
	os.WriteFile(filepath.Join(src, "helper.dll"), []byte("dll"), 0o644)
	tool, err := a.AddCustomTool(filepath.Join(src, "mytool.exe"), "Mio Tool", "Rete", "gui", true)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := loadManifest(base)
	got := m.find(tool.ID)
	if got == nil || !got.Custom || got.Category != "Rete" {
		t.Fatalf("lo strumento personale deve comparire nel catalogo: %+v", got)
	}
	if st := statusMap(a)[tool.ID]; st != "pronto" {
		t.Fatalf("appena aggiunto deve risultare sigillato e pronto, non %s", st)
	}
	if _, err := os.Stat(filepath.Join(base, "tools", tool.ID, "MyTool", "helper.dll")); err != nil {
		t.Fatal("con la cartella intera devono essere copiate anche le DLL")
	}
	if d, _ := a.verify(); !d.Valid {
		t.Fatalf("il sigillo deve includere lo strumento aggiunto: %+v", d)
	}
	if _, err := a.AddCustomTool(filepath.Join(base, "tools", "clitool", "cli.exe"), "Dentro", "", "", false); err == nil {
		t.Fatal("non si devono aggiungere file già dentro AFIP Portable")
	}
	if err := a.RemoveCustomTool("clitool"); err == nil {
		t.Fatal("non si devono poter rimuovere gli strumenti del catalogo")
	}
	if err := a.RemoveCustomTool(tool.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := loadManifest(base); m.find(tool.ID) != nil {
		t.Fatal("rimosso, non deve più comparire")
	}
	if d, _ := a.verify(); !d.Valid {
		t.Fatalf("dopo la rimozione il sigillo deve restare coerente: %+v", d)
	}
	all, _ := a.Ledger.readAll()
	var acts []string
	for _, e := range all {
		acts = append(acts, e.Action)
	}
	if !strings.Contains(strings.Join(acts, " "), "custom.add") || !strings.Contains(strings.Join(acts, " "), "custom.remove") {
		t.Fatalf("aggiunta e rimozione devono essere registrate: %v", acts)
	}
}

func TestPEKind(t *testing.T) {
	for path, want := range map[string]string{"/tmp/fake/fake.exe": "gui", "/tmp/ctest/ctest.exe": "cli"} {
		if _, err := os.Stat(path); err != nil {
			t.Skip("eseguibili di prova non presenti")
		}
		if got := peKind(path); got != want {
			t.Errorf("%s: atteso %s, ottenuto %s", path, want, got)
		}
	}
	if peKind("x.cmd") != "cli" || peKind("x.html") != "gui" {
		t.Fatal("estensioni")
	}
}

// Test reale contro GitHub (salta se la rete non c'e'): controlla e installa evtx_dump in una pendrive vuota.
func TestRealGitHubUpdate(t *testing.T) {
	if os.Getenv("AFIP_NET_TEST") == "" {
		t.Skip("impostare AFIP_NET_TEST=1 per il test di rete")
	}
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "manifest"), 0o755)
	cat, _ := os.ReadFile("../manifest/tools.json")
	os.WriteFile(filepath.Join(base, "manifest", "tools.json"), cat, 0o644)
	a := newApp(base, &fakeLauncher{})
	m, _ := loadManifest(base)
	for _, id := range []string{"evtx_dump", "pe-sieve"} {
		tl := m.find(id)
		inf := a.checkUpdate(tl)
		if inf.Status != "non installato" || inf.Latest == "" {
			t.Fatalf("%s: %+v", id, inf)
		}
		if err := a.updateTool(tl, inf, func(string) {}); err != nil {
			t.Fatal(err)
		}
		if again := a.checkUpdate(tl); again.Status != "aggiornato" {
			t.Fatalf("%s dopo l'installazione: %+v", id, again)
		}
	}
	if d, _ := a.verify(); !d.Valid {
		t.Fatalf("sigillo: %+v", d)
	}
	for _, p := range a.loadProvenance() {
		if !p.Verified {
			t.Errorf("%s: digest non verificato", p.File)
		}
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	f, _ := os.Create(path)
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
}

func TestExtractTarGzAndSlip(t *testing.T) {
	dir := t.TempDir()
	tgz := filepath.Join(dir, "py.tar.gz")
	writeTarGz(t, tgz, map[string]string{"python/python.exe": "MZ", "python/Lib/site-packages/README.txt": "x"})
	out := filepath.Join(dir, "out")
	os.MkdirAll(out, 0o755)
	if err := extract(tgz, out, "tar.gz", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "python", "python.exe")); err != nil {
		t.Fatal("estrazione tar.gz non riuscita")
	}
	bad := filepath.Join(dir, "bad.tar.gz")
	writeTarGz(t, bad, map[string]string{"../../evil.txt": "x"})
	if err := extract(bad, out, "tar.gz", ""); err == nil {
		t.Fatal("percorso fuori cartella non bloccato nel tar")
	}
}

func TestPythonToolsMissingWithoutPython(t *testing.T) {
	a, base := setup(t, &fakeLauncher{})
	cat := `{"schema":2,"categories":["M"],"tools":[{"id":"volatility3","name":"Volatility 3","category":"M","kind":"cli","availability":"python","exe":["vol.cmd"],"search_dirs":["runtime/bin"],"fetch":{"pip":["volatility3"]}}]}`
	os.WriteFile(filepath.Join(base, "manifest", "tools.json"), []byte(cat), 0o644)
	if st := statusMap(a)["volatility3"]; st != "mancante" {
		t.Fatalf("senza Python Volatility deve risultare mancante, non %s", st)
	}
	// Python installato dagli aggiornamenti in tools/python/<versione>/python, con il pacchetto
	sp := filepath.Join(base, "tools", "python", "20261020", "python", "Lib", "site-packages", "volatility3-2.28.2.dist-info")
	os.MkdirAll(sp, 0o755)
	exe := "python3"
	if runtime.GOOS == "windows" {
		exe = "python.exe"
	}
	os.WriteFile(filepath.Join(base, "tools", "python", "20261020", "python", exe), []byte("py"), 0o755)
	if a.pipInstalled("volatility3") != "2.28.2" {
		t.Fatalf("versione installata non letta: %q", a.pipInstalled("volatility3"))
	}
	if st := statusMap(a)["volatility3"]; st != "non_sigillato" {
		t.Fatalf("con Python e pacchetto deve risultare presente, non %s", st)
	}
	if a.pythonPrefix() != "tools/python/" {
		t.Fatal("prefisso del sigillo errato")
	}
}

func TestRealPythonRuntimeInstall(t *testing.T) {
	if os.Getenv("AFIP_NET_TEST") == "" {
		t.Skip("impostare AFIP_NET_TEST=1")
	}
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "manifest"), 0o755)
	cat, _ := os.ReadFile("../manifest/tools.json")
	os.WriteFile(filepath.Join(base, "manifest", "tools.json"), cat, 0o644)
	a := newApp(base, &fakeLauncher{})
	m, _ := loadManifest(base)
	py := m.find("python")
	inf := a.checkUpdate(py)
	if inf.Latest == "" || inf.Status != "non installato" {
		t.Fatalf("%+v", inf)
	}
	if err := a.updateTool(py, inf, func(string) {}); err != nil {
		t.Fatal(err)
	}
	hits, _ := filepath.Glob(filepath.Join(base, "tools", "python", "*", "python", "python.exe"))
	if len(hits) != 1 {
		t.Fatalf("python.exe non trovato: %v", hits)
	}
	if d, _ := a.verify(); !d.Valid {
		t.Fatalf("sigillo: %+v", d)
	}
	t.Logf("installato %s in %s", inf.Latest, hits[0])
}

//go:build !windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

// Fuori da Windows resta solo la riga di comando (sviluppo e test).
type systemLauncher struct{}

func (systemLauncher) GUI(exe string, args []string, dir string, env []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir, cmd.Env = dir, env
	return startDetached(cmd)
}
func (systemLauncher) Console(title, hint, dir string, pathDirs []string, env []string, scratch string) error {
	return errors.New("la console e' disponibile solo su Windows")
}
func (systemLauncher) OpenURL(string) error    { return errors.New("solo Windows") }
func (systemLauncher) OpenFolder(string) error { return errors.New("solo Windows") }

func main() {
	base := flag.String("base", baseDir(), "cartella radice")
	seal := flag.Bool("seal", false, "sigilla il toolkit")
	flag.Bool("verify", true, "verifica toolkit e registro")
	check := flag.String("check", "", "controlla gli aggiornamenti: all oppure id1,id2")
	update := flag.String("update", "", "installa o aggiorna: all oppure id1,id2")
	flag.Parse()
	a := newApp(*base, systemLauncher{})
	switch {
	case *update != "":
		os.Exit(runUpdateCLI(a, *update, true))
	case *check != "":
		os.Exit(runUpdateCLI(a, *check, false))
	}
	fmt.Println("AFIP Portable", version, "- l'interfaccia grafica e' disponibile solo su Windows.")
	os.Exit(runCLI(a, *seal))
}

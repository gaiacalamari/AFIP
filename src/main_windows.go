//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
)

func init() { runtime.LockOSThread() } // l'interfaccia Win32 deve restare sullo stesso thread

func main() {
	base := flag.String("base", baseDir(), "cartella radice di AFIP Portable")
	sealOnly := flag.Bool("seal", false, "sigilla il toolkit e esce")
	verifyOnly := flag.Bool("verify", false, "verifica toolkit e registro e esce (codice 0 = integro)")
	check := flag.String("check", "", "controlla gli aggiornamenti (all oppure id1,id2) e esce")
	update := flag.String("update", "", "installa o aggiorna (all oppure id1,id2) e esce")
	flag.Parse()
	for _, d := range []string{"tools", "logs", "manifest", "output"} {
		os.MkdirAll(*base+string(os.PathSeparator)+d, 0o755)
	}
	a := newApp(*base, systemLauncher{})
	if *sealOnly || *verifyOnly || *check != "" || *update != "" {
		// Eseguibile "grafico": per scrivere nella console da cui e' stato lanciato bisogna agganciarla.
		pAttachConsole.Call(^uintptr(0))
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stdout, os.Stderr = f, f
		}
		fmt.Println()
		switch {
		case *update != "":
			os.Exit(runUpdateCLI(a, *update, true))
		case *check != "":
			os.Exit(runUpdateCLI(a, *check, false))
		}
		os.Exit(runCLI(a, *sealOnly))
	}
	runGUI(a)
}

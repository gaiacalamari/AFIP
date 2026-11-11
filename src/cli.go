package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// runCLI: sigillo o verifica da riga di comando (usato anche dallo script di setup).
func runCLI(a *App, seal bool) int {
	if seal {
		s, err := a.seal()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Sigillo non riuscito:", err)
			return 1
		}
		fmt.Printf("Toolkit sigillato: %d file, impronta %s\n", s.Count, s.Digest)
		return 0
	}
	d, err := a.verify()
	c := a.Ledger.Verify()
	b, _ := json.MarshalIndent(map[string]any{"toolkit": d, "log": c, "error": errString(err)}, "", "  ")
	fmt.Println(string(b))
	if err != nil || !c.Valid || (d != nil && d.Sealed && !d.Valid) {
		return 2
	}
	return 0
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// runUpdateCLI: "-check" mostra le versioni, "-update all" o "-update id1,id2" installa/aggiorna.
func runUpdateCLI(a *App, which string, apply bool) int {
	m, err := loadManifest(a.Base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	want := map[string]bool{}
	for _, id := range strings.Split(which, ",") {
		if id = strings.TrimSpace(id); id != "" {
			want[id] = true
		}
	}
	code := 0
	for _, t := range a.updatable(m) {
		if !want["all"] && !want[t.ID] {
			continue
		}
		inf := a.checkUpdate(t)
		fmt.Printf("%-22s installata %-14s disponibile %-14s %s %s\n", t.ID, orNone(inf.Installed), inf.Latest, inf.Status, inf.Err)
		if inf.Status == "errore" {
			code = 1
			continue
		}
		if apply && inf.Status != "aggiornato" {
			if err := a.updateTool(t, inf, func(s string) { fmt.Printf("    %s\n", s) }); err != nil {
				fmt.Printf("    ERRORE: %v\n", err)
				code = 1
			} else {
				fmt.Printf("    installato %s\n", inf.Latest)
			}
		}
	}
	return code
}

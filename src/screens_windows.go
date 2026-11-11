//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// post esegue fn sul thread dell'interfaccia (i lavori in background non toccano mai le finestre direttamente).
func (u *ui) post(fn func()) {
	u.mu.Lock()
	u.queue = append(u.queue, fn)
	u.mu.Unlock()
	pPostMessageW.Call(u.hwnd, wmRunUI, 0, 0)
}

func (u *ui) drainQueue() {
	u.mu.Lock()
	q := u.queue
	u.queue = nil
	u.mu.Unlock()
	for _, fn := range q {
		fn()
	}
}

// --- Aggiornamenti software --------------------------------------------------------------------
func (u *ui) openUpdates() {
	if u.updWin != nil && u.updWin.hwnd != 0 {
		pShowWindow.Call(u.updWin.hwnd, swRestore)
		pSetForegroundWindow.Call(u.updWin.hwnd)
		return
	}
	m, err := loadManifest(u.app.Base)
	if err != nil {
		messageBox(u.hwnd, err.Error(), "Aggiornamenti", mbIconError)
		return
	}
	tools := u.app.updatable(m)
	prov := loadProvenance(u.app.Base)
	lw := &listWin{title: "Aggiornamenti software",
		info: "Cerca le nuove versioni ufficiali su GitHub (PyPI per Volatility 3 e MVT). Ogni file scaricato viene confrontato con l'SHA-256 pubblicato da GitHub, " +
			"installato al posto della versione precedente e aggiunto al sigillo; ogni aggiornamento finisce nel registro. Serve internet: usarlo sulla propria postazione.",
		cols: []lwCol{{"Strumento", 0.34}, {"Installata", 0.2}, {"Disponibile", 0.2}, {"Stato", 0.26}}}
	idx := map[string]int{}
	for _, t := range tools {
		idx[t.ID] = len(lw.rows)
		lw.rows = append(lw.rows, lwRow{id: t.ID, cols: []string{t.Name, orDash(u.app.installedVersion2(t, prov)), "…", "da controllare"},
			colors: []uintptr{0, cMuted, cMuted, cMuted}})
	}
	busy := false
	anyChecked := func() bool {
		for _, r := range lw.rows {
			if r.checkable && r.checked {
				return true
			}
		}
		return false
	}
	var check func()
	check = func() {
		busy = true
		lw.status, lw.stColor = "Controllo delle versioni in corso…", cAmber
		for i := range lw.rows {
			lw.rows[i].cols[2], lw.rows[i].cols[3], lw.rows[i].colors[3] = "…", "controllo…", cMuted
		}
		lw.redraw()
		go func() {
			up := newUpdater()
			u.app.CheckUpdates(up, tools, func(inf UpdateInfo) {
				u.post(func() {
					r := &lw.rows[idx[inf.ID]]
					r.cols[1], r.cols[2] = orDash(inf.Installed), orDash(inf.Latest)
					r.checkable, r.checked = false, false
					switch inf.Status {
					case "aggiornato":
						r.cols[3], r.colors[3] = "aggiornato", cOK
					case "disponibile", "da installare":
						r.cols[3], r.colors[3], r.colors[2] = inf.Status, cAmber, cAmber
						r.checkable, r.checked = true, true
					default:
						r.cols[3], r.colors[3] = "errore: "+inf.Err, cBad
					}
					lw.redraw()
				})
			})
			u.post(func() {
				busy = false
				n, e := 0, 0
				for _, r := range lw.rows {
					if r.checkable {
						n++
					}
					if strings.HasPrefix(r.cols[3], "errore") {
						e++
					}
				}
				switch {
				case e == len(lw.rows):
					lw.status, lw.stColor = "Nessuna risposta da GitHub: verificare la connessione o il proxy.", cBad
				case n == 0:
					lw.status, lw.stColor = "Tutti gli strumenti sono all'ultima versione.", cOK
				default:
					lw.status, lw.stColor = plural(n, "aggiornamento disponibile", "aggiornamenti disponibili")+": selezionati in cima all'elenco, premere \"Aggiorna selezionati\".", cAmber
				}
				// in cima quelli da aggiornare, poi gli errori, poi gli aggiornati
				rank := func(r lwRow) int {
					switch {
					case r.checkable:
						return 0
					case strings.HasPrefix(r.cols[3], "errore"):
						return 1
					}
					return 2
				}
				sort.SliceStable(lw.rows, func(i, j int) bool { return rank(lw.rows[i]) < rank(lw.rows[j]) })
				for i, r := range lw.rows {
					idx[r.id] = i
				}
				lw.scroll, lw.sel = 0, -1
				if e > 0 && e < len(lw.rows) {
					lw.status += " (" + plural(e, "errore", "errori") + ")"
				}
				lw.redraw()
			})
		}()
	}
	update := func() {
		var sel []int
		for i, r := range lw.rows {
			if r.checkable && r.checked {
				sel = append(sel, i)
			}
		}
		if messageBox(lw.hwnd, fmt.Sprintf("Aggiornare %s?\n\nI programmi da aggiornare devono essere chiusi. La versione installata viene sostituita solo se il download è integro.", plural(len(sel), "strumento", "strumenti")),
			"Aggiornamenti software", mbOKCancel|mbIconQuest) != idOK {
			return
		}
		busy = true
		go func() {
			up := newUpdater()
			ok, ko := 0, 0
			for _, i := range sel {
				i := i
				id := lw.rows[i].id
				t := m.find(id)
				u.post(func() { lw.rows[i].cols[3], lw.rows[i].colors[3] = "in corso…", cAmber; lw.redraw() })
				tag, err := u.app.UpdateTool(up, t, func(s string) {
					u.post(func() { lw.rows[i].cols[3] = s; lw.status = t.Name + ": " + s; lw.redraw() })
				})
				if err != nil {
					ko++
				} else {
					ok++
				}
				u.post(func() {
					r := &lw.rows[i]
					r.checkable, r.checked = false, false
					if err != nil {
						r.cols[3], r.colors[3] = "errore: "+err.Error(), cBad
					} else {
						r.cols[1], r.cols[3], r.colors[1], r.colors[2], r.colors[3] = tag, "installato", cText, cMuted, cOK
					}
					lw.redraw()
				})
			}
			u.post(func() {
				busy = false
				lw.status, lw.stColor = "Aggiornamento completato: "+plural(ok, "strumento aggiornato", "strumenti aggiornati")+", "+plural(ko, "errore", "errori")+".", map[bool]uintptr{true: cOK, false: cBad}[ko == 0]
				lw.redraw()
				u.reloadManifest()
				u.say(lw.status, lw.stColor)
			})
		}()
	}
	lw.buttons = []lwBtn{
		{label: "Seleziona tutti", enabled: func() bool { return !busy }, click: func() {
			all := anyChecked()
			for i := range lw.rows {
				if lw.rows[i].checkable {
					lw.rows[i].checked = !all
				}
			}
		}},
		{label: "Controlla di nuovo", enabled: func() bool { return !busy }, click: check},
		{label: "Aggiorna selezionati", primary: true, enabled: func() bool { return !busy && anyChecked() }, click: update},
		{label: "Chiudi", click: func() { pPostMessageW.Call(lw.hwnd, wmClose, 0, 0) }},
	}
	lw.canClose = func() bool {
		return !busy || messageBox(lw.hwnd, "Un'operazione è ancora in corso. Chiudere comunque la finestra? L'operazione prosegue in background.", "Aggiornamenti software", mbYesNo|mbIconWarn) == idYes
	}
	u.updWin = lw
	u.openListWin(lw, 760, 560)
	check()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- Strumenti personali -----------------------------------------------------------------------
func (u *ui) openCustom() {
	if u.custWin != nil && u.custWin.hwnd != 0 {
		pShowWindow.Call(u.custWin.hwnd, swRestore)
		pSetForegroundWindow.Call(u.custWin.hwnd)
		return
	}
	lw := &listWin{title: "Strumenti personali",
		info: "Programmi aggiunti da te. Vengono copiati nella pendrive (tools\\custom-…), compaiono nel pannello nella categoria scelta, " +
			"sono controllati dal sigillo come gli altri e ogni aggiunta o rimozione finisce nel registro.",
		cols: []lwCol{{"Nome", 0.3}, {"Categoria", 0.24}, {"Tipo", 0.14}, {"File", 0.32}}}
	busy := false
	refresh := func() {
		lw.rows = lw.rows[:0]
		for _, t := range loadCustom(u.app.Base).Tools {
			kind := "finestra"
			if t.Kind == "cli" {
				kind = "console"
			}
			file := "—"
			if len(t.Exe) > 0 {
				file = t.Exe[0]
			}
			lw.rows = append(lw.rows, lwRow{id: t.ID, cols: []string{t.Name, t.Category, kind, file}, colors: []uintptr{0, cMuted, cMuted, cMuted}})
		}
		if lw.sel >= len(lw.rows) {
			lw.sel = -1
		}
		if len(lw.rows) == 0 {
			lw.status, lw.stColor = "Nessuno strumento personale. Premi \"Aggiungi eseguibile…\".", cMuted
		}
		lw.redraw()
	}
	add := func() {
		path, ok := openFileDialog(lw.hwnd, "Scegli il programma da aggiungere ad AFIP Portable")
		if !ok {
			return
		}
		if rel, err := filepath.Rel(u.app.Base, path); err == nil && !strings.HasPrefix(rel, "..") {
			messageBox(lw.hwnd, "Il file è già dentro la cartella di AFIP Portable: sceglierne uno esterno.", "Strumenti personali", mbIconWarn)
			return
		}
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		name, ok := u.inputBoxOwner(lw.hwnd, "Nome dello strumento", "Nome con cui comparirà nel pannello.", base, 2)
		if !ok {
			return
		}
		cat, ok := u.inputBoxOwner(lw.hwnd, "Categoria", "Categoria del pannello in cui inserirlo (esistente o nuova).", customCategory, 2)
		if !ok {
			return
		}
		kind := peKind(path)
		if kind == "cli" {
			if messageBox(lw.hwnd, filepath.Base(path)+" è un programma da riga di comando.\n\nSì: aprirlo in una console con il programma nel PATH (consigliato)\nNo: avviarlo direttamente", "Tipo di avvio", mbYesNo|mbIconQuest) == idNo {
				kind = "gui"
			}
		}
		size, n := dirSize(filepath.Dir(path))
		whole := false
		switch messageBox(lw.hwnd, fmt.Sprintf("Copiare nella pendrive l'intera cartella che contiene il programma?\n\n%s\n%d file, %.1f MB\n\nSì: cartella intera (consigliato se il programma usa DLL o file accanto)\nNo: solo %s",
			filepath.Dir(path), n, float64(size)/1048576, filepath.Base(path)), "Cosa copiare", mbYesNoCancel|mbIconQuest) {
		case idYes:
			whole = true
			if size > 2<<30 && messageBox(lw.hwnd, "La cartella supera i 2 GB. Continuare?", "Cosa copiare", mbYesNo|mbIconWarn) != idYes {
				return
			}
		case idNo:
		default:
			return
		}
		busy = true
		lw.status, lw.stColor = "Copia e calcolo degli hash in corso…", cAmber
		go func() {
			t, err := u.app.AddCustomTool(path, name, cat, kind, whole)
			u.post(func() {
				busy = false
				if err != nil {
					lw.status, lw.stColor = "Aggiunta non riuscita: "+err.Error(), cBad
					messageBox(lw.hwnd, err.Error(), "Strumenti personali", mbIconError)
				} else {
					lw.status, lw.stColor = t.Name+" aggiunto nella categoria "+t.Category+".", cOK
				}
				refresh()
				u.reloadManifest()
				u.redraw()
			})
		}()
	}
	remove := func() {
		if lw.sel < 0 || lw.sel >= len(lw.rows) {
			return
		}
		r := lw.rows[lw.sel]
		if messageBox(lw.hwnd, "Rimuovere "+r.cols[0]+" dagli strumenti personali?\n\nLa sua cartella nella pendrive verrà cancellata (il file originale non viene toccato).", "Strumenti personali", mbYesNo|mbIconQuest) != idYes {
			return
		}
		if err := u.app.RemoveCustomTool(r.id); err != nil {
			messageBox(lw.hwnd, err.Error(), "Strumenti personali", mbIconError)
		} else {
			lw.status, lw.stColor = r.cols[0]+" rimosso.", cOK
		}
		refresh()
		u.reloadManifest()
		u.redraw()
	}
	lw.buttons = []lwBtn{
		{label: "Apri cartella", enabled: func() bool { return lw.sel >= 0 && !busy }, click: func() {
			if lw.sel >= 0 {
				shellOpen(lw.hwnd, filepath.Join(u.app.Base, "tools", lw.rows[lw.sel].id), "")
			}
		}},
		{label: "Rimuovi", enabled: func() bool { return lw.sel >= 0 && !busy }, click: remove},
		{label: "Aggiungi eseguibile…", primary: true, enabled: func() bool { return !busy }, click: add},
		{label: "Chiudi", click: func() { pPostMessageW.Call(lw.hwnd, wmClose, 0, 0) }},
	}
	lw.canClose = func() bool { return !busy }
	u.custWin = lw
	u.openListWin(lw, 720, 460)
	refresh()
}

// removeCustomFromPanel: voce del tasto destro sugli strumenti personali.
func (u *ui) removeCustomFromPanel(st *ToolState) {
	if messageBox(u.hwnd, "Rimuovere "+st.Name+" dagli strumenti personali?\n\nLa sua cartella nella pendrive verrà cancellata.", "Strumenti personali", mbYesNo|mbIconQuest) != idYes {
		return
	}
	if err := u.app.RemoveCustomTool(st.ID); err != nil {
		messageBox(u.hwnd, err.Error(), "Strumenti personali", mbIconError)
		return
	}
	u.reloadManifest()
	u.say(st.Name+" rimosso dagli strumenti personali", cOK)
	if u.custWin != nil && u.custWin.hwnd != 0 {
		pPostMessageW.Call(u.custWin.hwnd, wmClose, 0, 0)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

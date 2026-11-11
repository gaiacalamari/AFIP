//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Palette (COLORREF = 0x00BBGGRR)
var (
	cInk    = rgb(0x10, 0x14, 0x1c)
	cPanel  = rgb(0x17, 0x1d, 0x29)
	cRaise  = rgb(0x1e, 0x26, 0x36)
	cLine   = rgb(0x2b, 0x34, 0x46)
	cText   = rgb(0xdc, 0xe1, 0xea)
	cMuted  = rgb(0x8e, 0x98, 0xab)
	cAmber  = rgb(0xd9, 0xa4, 0x41)
	cOK     = rgb(0x5d, 0xb8, 0xa6)
	cBad    = rgb(0xe0, 0x6a, 0x5f)
	cInfo   = rgb(0x7f, 0xa6, 0xe0)
	cOnAmbr = rgb(0x1a, 0x13, 0x06)
)

const (
	className  = "AFIPPortable10"
	wmStates   = wmApp + 1 // stati con hash calcolati in background
	wmJobDone  = wmApp + 2 // sigillo o verifica terminati
	wmRunUI    = wmApp + 3 // funzioni accodate dai lavori in background
	editID     = 1001
	headerH    = 44
	searchH    = 40
	footerH    = 54
	catRowH    = 30
	toolRowH   = 42
	minW, minH = 260, 320
)

// Pulsanti dell'intestazione, da destra verso sinistra.
const (
	btnClose = iota
	btnCompact
	btnPin
	btnMenu
	btnCount
)

type row struct {
	header bool
	cat    string
	idx    int // indice in states
	y, h   int32
}

type ui struct {
	app      *App
	hwnd     uintptr
	edit     uintptr
	editOrig uintptr
	dpi      int32
	fTitle   uintptr
	fName    uintptr
	fBody    uintptr
	fSmall   uintptr

	manifest *Manifest
	states   []ToolState
	hashed   bool
	logStat  ChainStatus
	settings Settings

	mu        sync.Mutex
	pending   []ToolState
	pendLog   ChainStatus
	jobResult func()

	rows      []row
	scroll    int32
	contentH  int32
	hoverRow  int
	hoverBtn  int
	hoverCase bool
	sel       int
	selID     string
	downAt    string
	tracking  bool
	message   string
	msgColor  uintptr
	busy      bool
	normalH   int32
	brushes   map[uintptr]uintptr
	queue     []func()
	updWin    *listWin
	custWin   *listWin
	pens      map[uintptr]uintptr
}

var theUI *ui

func (u *ui) s(v int32) int32 { return int32(math.Round(float64(v) * float64(u.dpi) / 96)) }

func (u *ui) brush(c uintptr) uintptr {
	if b, ok := u.brushes[c]; ok {
		return b
	}
	b, _, _ := pCreateSolidBrush.Call(c)
	u.brushes[c] = b
	return b
}

func (u *ui) pen(c uintptr) uintptr {
	if p, ok := u.pens[c]; ok {
		return p
	}
	p, _, _ := pCreatePen.Call(0, uintptr(u.s(1)), c)
	u.pens[c] = p
	return p
}

func (u *ui) makeFonts() {
	for _, f := range []uintptr{u.fTitle, u.fName, u.fBody, u.fSmall} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	mk := func(px int32, weight int, face string) uintptr {
		f, _, _ := pCreateFontW.Call(uintptr(-u.s(px)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, u16p(face))
		return f
	}
	u.fTitle = mk(15, 600, "Segoe UI Semibold")
	u.fName = mk(13, 600, "Segoe UI Semibold")
	u.fBody = mk(13, 400, "Segoe UI")
	u.fSmall = mk(11, 400, "Segoe UI")
	if u.edit != 0 {
		pSendMessageW.Call(u.edit, wmSetFont, u.fBody, 1)
	}
}

// --- dati -------------------------------------------------------------------------------------
func (u *ui) reloadManifest() {
	m, err := loadManifest(u.app.Base)
	if err != nil {
		messageBox(u.hwnd, "Impossibile leggere manifest\\tools.json:\n"+err.Error(), "AFIP Portable", mbIconError)
		m = &Manifest{}
	}
	u.manifest = m
	u.states = u.app.states(m, false) // primo disegno immediato, senza hash
	u.hashed = false
	u.logStat = u.app.Ledger.Verify()
	u.layout()
	u.refreshAsync()
}

func (u *ui) refreshAsync() {
	m := u.manifest
	go func() {
		st := u.app.states(m, true)
		lg := u.app.Ledger.Verify()
		u.mu.Lock()
		u.pending, u.pendLog = st, lg
		u.mu.Unlock()
		pPostMessageW.Call(u.hwnd, wmStates, 0, 0)
	}()
}

func (u *ui) visible(st *ToolState) bool {
	if st.Runtime { // il Python portatile si gestisce dagli aggiornamenti, non si avvia dal pannello
		return false
	}
	if !u.settings.ShowUnavailable && st.Status == "non_disponibile" {
		return false
	}
	if u.settings.OnlyPresent && st.ExePath == "" && st.Status != "url" {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(getText(u.edit)))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(st.Name+" "+st.Description+" "+st.Category+" "+st.ID), q)
}

func (u *ui) layout() {
	u.rows = u.rows[:0]
	searching := strings.TrimSpace(getText(u.edit)) != ""
	y := int32(0)
	for _, c := range u.manifest.Categories {
		var idx []int
		for i := range u.states {
			if u.states[i].Category == c && u.visible(&u.states[i]) {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		u.rows = append(u.rows, row{header: true, cat: c, y: y, h: u.s(catRowH)})
		y += u.s(catRowH)
		if u.settings.Collapsed[c] && !searching {
			continue
		}
		for _, i := range idx {
			u.rows = append(u.rows, row{cat: c, idx: i, y: y, h: u.s(toolRowH)})
			y += u.s(toolRowH)
		}
	}
	u.contentH = y
	u.clampScroll()
	// la selezione segue lo strumento, non la posizione: aprire o chiudere una categoria non la sposta
	u.sel = -1
	for i, r := range u.rows {
		if !r.header && u.states[r.idx].ID == u.selID {
			u.sel = i
		}
	}
	if u.sel < 0 {
		u.setSel(u.firstTool())
	}
}

func (u *ui) setSel(i int) {
	u.sel, u.selID = i, ""
	if i >= 0 && i < len(u.rows) && !u.rows[i].header {
		u.selID = u.states[u.rows[i].idx].ID
	}
}

func (u *ui) firstTool() int {
	for i, r := range u.rows {
		if !r.header {
			return i
		}
	}
	return -1
}

// --- geometria --------------------------------------------------------------------------------
func (u *ui) client() rect {
	var r rect
	pGetClientRect.Call(u.hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func (u *ui) listRect() rect {
	c := u.client()
	return rect{0, u.s(headerH + searchH), c.R, c.B - u.s(footerH)}
}

func (u *ui) clampScroll() {
	lr := u.listRect()
	maxS := u.contentH - (lr.B - lr.T)
	if maxS < 0 {
		maxS = 0
	}
	if u.scroll > maxS {
		u.scroll = maxS
	}
	if u.scroll < 0 {
		u.scroll = 0
	}
}

func (u *ui) btnRect(i int) rect {
	c := u.client()
	w := u.s(30)
	r := c.R - u.s(8) - int32(i+1)*w
	return rect{r, u.s(7), r + w, u.s(7) + w}
}

func (u *ui) placeEdit() {
	c := u.client()
	x, y := u.s(22), u.s(headerH)+u.s(10)
	w, h := c.R-u.s(44), u.s(20)
	if u.settings.Compact {
		pShowWindow.Call(u.edit, 0)
		return
	}
	pShowWindow.Call(u.edit, swShow)
	pMoveWindow.Call(u.edit, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

func inRect(r rect, x, y int32) bool { return x >= r.L && x < r.R && y >= r.T && y < r.B }

func (u *ui) rowAt(x, y int32) int {
	lr := u.listRect()
	if !inRect(lr, x, y) {
		return -1
	}
	cy := y - lr.T + u.scroll
	for i, r := range u.rows {
		if cy >= r.y && cy < r.y+r.h {
			return i
		}
	}
	return -1
}

func (u *ui) btnAt(x, y int32) int {
	for i := 0; i < btnCount; i++ {
		if inRect(u.btnRect(i), x, y) {
			return i
		}
	}
	return -1
}

// --- disegno ----------------------------------------------------------------------------------
type painter struct {
	u  *ui
	dc uintptr
}

func (p painter) fill(r rect, c uintptr) {
	pFillRect.Call(p.dc, uintptr(unsafe.Pointer(&r)), p.u.brush(c))
}

func (p painter) round(r rect, rad int32, fill, border uintptr, hasBorder bool) {
	pSelectObject.Call(p.dc, p.u.brush(fill))
	if hasBorder {
		pSelectObject.Call(p.dc, p.u.pen(border))
	} else {
		np, _, _ := pGetStockObject.Call(nullPen)
		pSelectObject.Call(p.dc, np)
	}
	pRoundRect.Call(p.dc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B), uintptr(rad), uintptr(rad))
}

func (p painter) text(s string, r rect, font, color uintptr, flags uintptr) {
	pSelectObject.Call(p.dc, font)
	pSetTextColor.Call(p.dc, color)
	ws, _ := syscall.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	pDrawTextW.Call(p.dc, uintptr(unsafe.Pointer(&ws[0])), uintptr(len(ws)-1), uintptr(unsafe.Pointer(&r)), flags|dtNoPrefix)
}

func (p painter) line(x1, y1, x2, y2 int32, c uintptr, width int32) {
	pen, _, _ := pCreatePen.Call(0, uintptr(width), c)
	old, _, _ := pSelectObject.Call(p.dc, pen)
	pMoveToEx.Call(p.dc, uintptr(x1), uintptr(y1), 0)
	pLineTo.Call(p.dc, uintptr(x2), uintptr(y2))
	pSelectObject.Call(p.dc, old)
	pDeleteObject.Call(pen)
}

func (p painter) dot(cx, cy, r int32, fill, border uintptr, hollow bool) {
	if hollow {
		nb, _, _ := pGetStockObject.Call(nullBrsh)
		pSelectObject.Call(p.dc, nb)
	} else {
		pSelectObject.Call(p.dc, p.u.brush(fill))
	}
	pSelectObject.Call(p.dc, p.u.pen(border))
	pEllipse.Call(p.dc, uintptr(cx-r), uintptr(cy-r), uintptr(cx+r+1), uintptr(cy+r+1))
}

func (p painter) poly(c uintptr, pts ...point) {
	pSelectObject.Call(p.dc, p.u.brush(c))
	np, _, _ := pGetStockObject.Call(nullPen)
	pSelectObject.Call(p.dc, np)
	pPolygon.Call(p.dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
}

// Icone vettoriali disegnate con GDI: non dipendono dai font di sistema.
func (p painter) icon(kind int, r rect, c uintptr, on bool) {
	u := p.u
	cx, cy := (r.L+r.R)/2, (r.T+r.B)/2
	w := u.s(1) + u.s(1)/2
	switch kind {
	case btnClose:
		d := u.s(5)
		p.line(cx-d, cy-d, cx+d+1, cy+d+1, c, w)
		p.line(cx+d, cy-d, cx-d-1, cy+d+1, c, w)
	case btnCompact:
		d := u.s(5)
		if on { // compatto: freccia verso il basso per espandere
			p.line(cx-d, cy-d/2, cx, cy+d/2, c, w)
			p.line(cx, cy+d/2, cx+d, cy-d/2, c, w)
		} else {
			p.line(cx-d, cy+d/2, cx, cy-d/2, c, w)
			p.line(cx, cy-d/2, cx+d, cy+d/2, c, w)
		}
	case btnPin:
		rr := u.s(4)
		p.dot(cx, cy-u.s(2), rr, cAmber, c, !on)
		if on {
			p.dot(cx, cy-u.s(2), rr, cAmber, cAmber, false)
		}
		p.line(cx, cy+u.s(2), cx, cy+u.s(7), c, w)
	case btnMenu:
		d := u.s(6)
		for i := int32(-1); i <= 1; i++ {
			p.line(cx-d, cy+i*u.s(4), cx+d+1, cy+i*u.s(4), c, w)
		}
	}
}

func hsl(h, s, l float64) uintptr {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return rgb(byte((r+m)*255), byte((g+m)*255), byte((b+m)*255))
}

func statusInfo(st *ToolState) (label string, color uintptr, hollow bool) {
	switch st.Status {
	case "pronto":
		return "pronto", cOK, false
	case "non_sigillato":
		return "non sigillato", cInfo, true
	case "modificato":
		return "hash diverso dal sigillo", cBad, false
	case "presente":
		return "controllo dell'hash…", cMuted, true
	case "url":
		return "servizio di laboratorio", cInfo, false
	case "non_disponibile":
		if st.Availability == "deprecated" {
			return "non mantenuto", cLine, true
		}
		return "solo Linux", cLine, true
	default:
		return "non presente", cMuted, true
	}
}

func (u *ui) paint(dc uintptr) {
	c := u.client()
	p := painter{u, dc}
	pSetBkMode.Call(dc, 1)
	p.fill(c, cInk)

	// intestazione: zona di trascinamento
	hr := rect{0, 0, c.R, u.s(headerH)}
	p.fill(hr, cPanel)
	if icon := appIconSmall; icon != 0 {
		sz := u.s(22)
		pDrawIconEx.Call(dc, uintptr(u.s(12)), uintptr((u.s(headerH)-sz)/2), icon, uintptr(sz), uintptr(sz), 0, 0, 3)
	}
	titleR := rect{u.s(42), 0, u.btnRect(btnCount-1).L - u.s(4), u.s(headerH)}
	p.text("AFIP Portable", titleR, u.fTitle, cText, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	vr := titleR
	vr.L += u.s(108)
	p.text(version[:3], vr, u.fSmall, cMuted, dtLeft|dtVCenter|dtSingle)
	for i := 0; i < btnCount; i++ {
		br := u.btnRect(i)
		col := cMuted
		if u.hoverBtn == i {
			bg := cRaise
			if i == btnClose {
				bg = cBad
			}
			p.round(br, u.s(6), bg, 0, false)
			col = cText
		}
		on := (i == btnPin && u.settings.Topmost) || (i == btnCompact && u.settings.Compact)
		p.icon(i, br, col, on)
	}
	p.fill(rect{0, u.s(headerH) - 1, c.R, u.s(headerH)}, cLine)
	if u.settings.Compact {
		return // barra flottante: solo intestazione con i pulsanti
	}

	// campo di ricerca (controllo EDIT nativo dentro un riquadro disegnato)
	top := u.s(headerH)
	sr := rect{u.s(12), top + u.s(6), c.R - u.s(12), top + u.s(searchH) - u.s(6)}
	p.round(sr, u.s(6), cInk, cLine, true)

	// elenco
	lr := u.listRect()
	pIntersectClipRect.Call(dc, uintptr(lr.L), uintptr(lr.T), uintptr(lr.R), uintptr(lr.B))
	for i, r := range u.rows {
		top := lr.T + r.y - u.scroll
		if top+r.h < lr.T || top > lr.B {
			continue
		}
		rr := rect{0, top, c.R, top + r.h}
		if r.header {
			n, ready := 0, 0
			for j := range u.states {
				if u.states[j].Category == r.cat && u.visible(&u.states[j]) {
					n++
					if u.states[j].ExePath != "" || u.states[j].Status == "url" {
						ready++
					}
				}
			}
			if i == u.hoverRow {
				p.fill(rr, cPanel)
			}
			cx, cy, d := u.s(18), (rr.T+rr.B)/2, u.s(4)
			if u.settings.Collapsed[r.cat] && strings.TrimSpace(getText(u.edit)) == "" {
				p.poly(cMuted, point{cx - d/2, cy - d}, point{cx + d, cy}, point{cx - d/2, cy + d})
			} else {
				p.poly(cMuted, point{cx - d, cy - d/2}, point{cx + d, cy - d/2}, point{cx, cy + d})
			}
			p.text(r.cat, rect{u.s(30), rr.T, rr.R - u.s(60), rr.B}, u.fName, cMuted, dtLeft|dtVCenter|dtSingle|dtEllipsis)
			p.text(fmt.Sprintf("%d/%d", ready, n), rect{rr.R - u.s(70), rr.T, rr.R - u.s(14), rr.B}, u.fSmall, cMuted, dtRight|dtVCenter|dtSingle)
			continue
		}
		st := &u.states[r.idx]
		if i == u.hoverRow {
			p.round(rect{u.s(6), rr.T + u.s(2), rr.R - u.s(6), rr.B - u.s(2)}, u.s(6), cRaise, 0, false)
		}
		if i == u.sel {
			p.fill(rect{u.s(6), rr.T + u.s(8), u.s(9), rr.B - u.s(8)}, cAmber)
		}
		label, col, hollow := statusInfo(st)
		p.dot(u.s(22), rr.T+u.s(15), u.s(4), col, col, hollow)
		nameCol := cText
		if st.ExePath == "" && st.Status != "url" {
			nameCol = cMuted
		}
		p.text(st.Name, rect{u.s(34), rr.T + u.s(4), rr.R - u.s(12), rr.T + u.s(24)}, u.fName, nameCol, dtLeft|dtVCenter|dtSingle|dtEllipsis)
		sub := label
		if st.ExePath != "" {
			if st.Kind == "cli" {
				sub += " · console"
			} else {
				sub += " · grafico"
			}
		}
		subR := rect{u.s(34), rr.T + u.s(22), rr.R - u.s(12), rr.B - u.s(4)}
		if st.AuthRequired {
			p.text("autorizzazione", subR, u.fSmall, cAmber, dtRight|dtVCenter|dtSingle)
			subR.R -= u.s(88)
		}
		p.text(sub, subR, u.fSmall, cMuted, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	}
	if len(u.rows) == 0 {
		p.text("Nessuno strumento da mostrare: cambia la ricerca o i filtri nel menu ≡.", rect{u.s(14), lr.T + u.s(16), lr.R - u.s(14), lr.T + u.s(60)}, u.fBody, cMuted, dtLeft|dtWordBreak)
	}
	// barra di scorrimento sottile
	if vis := lr.B - lr.T; u.contentH > vis {
		th := vis * vis / u.contentH
		if th < u.s(24) {
			th = u.s(24)
		}
		ty := lr.T + (vis-th)*u.scroll/(u.contentH-vis)
		p.round(rect{c.R - u.s(6), ty, c.R - u.s(2), ty + th}, u.s(4), cLine, 0, false)
	}
	pSelectClipRgn.Call(dc, 0)

	// piede: messaggio e stato del registro
	fr := rect{0, c.B - u.s(footerH), c.R, c.B}
	p.fill(fr, cPanel)
	p.fill(rect{0, fr.T, c.R, fr.T + 1}, cLine)
	msgText, msgCol := u.message, u.msgColor
	if u.hoverRow >= 0 && u.hoverRow < len(u.rows) && !u.rows[u.hoverRow].header {
		st := &u.states[u.rows[u.hoverRow].idx]
		msgText, msgCol = st.Description, cMuted
	}
	if msgCol == 0 {
		msgCol = cMuted
	}
	p.text(msgText, rect{u.s(12), fr.T + u.s(6), fr.R - u.s(12), fr.T + u.s(26)}, u.fBody, msgCol, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	ly := fr.T + u.s(32)
	if u.logStat.Valid {
		h := u.logStat.Head
		x := u.s(12)
		for i := 0; i < 16 && len(h) >= 64; i++ {
			v, _ := strconv.ParseUint(h[i*4:i*4+4], 16, 32)
			col := hsl(float64(v)*360/65536, 0.22+float64(v%16)/100, 0.38+float64(v%22)/100)
			p.round(rect{x, ly, x + u.s(7), ly + u.s(12)}, u.s(2), col, 0, false)
			x += u.s(9)
		}
		p.text(fmt.Sprintf("registro integro · %d voci", u.logStat.Entries), rect{x + u.s(6), ly - u.s(2), fr.R - u.s(12), ly + u.s(14)}, u.fSmall, cOK, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	} else {
		p.text(fmt.Sprintf("REGISTRO INTERROTTO alla voce %d: non usare la pendrive finché non è verificata", u.logStat.BrokenAt), rect{u.s(12), ly - u.s(2), fr.R - u.s(12), ly + u.s(14)}, u.fSmall, cBad, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	}
}

func (u *ui) onPaint() {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	c := u.client()
	mem, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(c.R), uintptr(c.B))
	old, _, _ := pSelectObject.Call(mem, bmp)
	u.paint(mem)
	pBitBlt.Call(hdc, 0, 0, uintptr(c.R), uintptr(c.B), mem, 0, 0, srcCopy)
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
	pEndPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
}

func (u *ui) redraw() { pInvalidateRect.Call(u.hwnd, 0, 0) }

func (u *ui) say(text string, color uintptr) {
	u.message, u.msgColor = text, color
	u.redraw()
}

// --- azioni -----------------------------------------------------------------------------------
func (u *ui) toolAt(i int) *ToolState {
	if i < 0 || i >= len(u.rows) || u.rows[i].header {
		return nil
	}
	return &u.states[u.rows[i].idx]
}

func (u *ui) activate(i int, mode string) {
	if i < 0 || i >= len(u.rows) {
		return
	}
	if u.rows[i].header {
		c := u.rows[i].cat
		u.settings.Collapsed[c] = !u.settings.Collapsed[c]
		u.app.saveSettings(u.settings)
		u.layout()
		u.redraw()
		return
	}
	u.launchTool(u.toolAt(i), mode)
}

func (u *ui) launchTool(st *ToolState, mode string) {
	switch st.Status {
	case "non_disponibile":
		messageBox(u.hwnd, st.Name+" non si esegue su Windows.\n\n"+st.Alternative, "AFIP Portable", mbIconInfo)
		return
	case "mancante":
		what := "Si installa da menu ≡ → Aggiornamenti software (serve internet)."
		if st.Fetch["pip"] != nil {
			what = "Si installa da menu ≡ → Aggiornamenti software: prima il Python portatile, poi " + st.Name + "."
		} else if st.Custom {
			what = "Il file dello strumento personale non c'è più: aggiungerlo di nuovo da Strumenti personali."
		}
		r := messageBox(u.hwnd, fmt.Sprintf("%s non è presente sulla pendrive.\n\n%s\nCartella: %s\n\nSì: apri gli aggiornamenti   No: apri la pagina ufficiale", st.Name, what, st.ToolDir),
			"AFIP Portable", mbYesNoCancel|mbIconQuest)
		if r == idYes {
			u.openUpdates()
		} else if r == idNo && st.Homepage != "" {
			shellOpen(u.hwnd, st.Homepage, "")
		}
		return
	}
	if mode == "" {
		switch {
		case st.Status == "url":
			mode = "url"
		case st.Kind == "cli":
			mode = "console"
		default:
			mode = "gui"
		}
	}
	req := LaunchReq{ID: st.ID, Mode: mode}
	if st.AuthRequired {
		if messageBox(u.hwnd, st.Name+" accede a password e credenziali.\n\n"+st.Warning+"\n\nConfermi di essere autorizzato a usarlo in questa attività? La conferma viene annotata nel registro.",
			"Strumento per credenziali", mbYesNo|mbIconWarn) != idYes {
			return
		}
		req.Confirmed = true
	}
	for {
		e, err := u.app.launch(req)
		if err == ErrNeedReason {
			why, ok := u.inputBox(st.Name+": hash diverso dal sigillo",
				"L'eseguibile non corrisponde a quello sigillato. Se l'hai aggiornato tu, sigilla di nuovo il toolkit dal menu.\n\nPer avviarlo comunque scrivi una motivazione, che verrà registrata.", "", 4)
			if !ok {
				return
			}
			req.Force, req.Reason = true, why
			continue
		}
		if err != nil {
			u.say(st.Name+": "+err.Error(), cBad)
			messageBox(u.hwnd, st.Name+"\n\n"+err.Error(), "Avvio non riuscito", mbIconError)
		} else {
			u.say(fmt.Sprintf("%s avviato · voce %d del registro", st.Name, e.Seq), cOK)
		}
		break
	}
	u.refreshAsync()
}

func (u *ui) toolMenu(i int) {
	st := u.toolAt(i)
	if st == nil {
		return
	}
	m, _, _ := pCreatePopupMenu.Call()
	defer pDestroyMenu.Call(m)
	present := st.ExePath != ""
	add := func(id int, text string, enabled bool) {
		f := uintptr(mfString)
		if !enabled {
			f |= mfGrayed
		}
		pAppendMenuW.Call(m, f, uintptr(id), u16p(text))
	}
	add(1, "Avvia", present && st.Kind == "gui")
	add(2, "Apri console", present && st.ConsoleOK)
	if st.Status == "url" {
		add(6, "Apri "+st.URL, true)
	}
	pAppendMenuW.Call(m, mfSeparator, 0, 0)
	add(3, "Apri la cartella dello strumento", st.Status != "non_disponibile" && st.Status != "url")
	add(4, "Pagina ufficiale", st.Homepage != "")
	add(5, "Informazioni e hash…", true)
	if st.Custom {
		pAppendMenuW.Call(m, mfSeparator, 0, 0)
		add(7, "Rimuovi dagli strumenti personali…", true)
	}
	switch u.popup(m) {
	case 1:
		u.launchTool(st, "gui")
	case 2:
		u.launchTool(st, "console")
	case 6:
		u.launchTool(st, "url")
	case 3:
		os.MkdirAll(st.ToolDir, 0o755)
		shellOpen(u.hwnd, st.ToolDir, "")
	case 7:
		u.removeCustomFromPanel(st)
	case 4:
		shellOpen(u.hwnd, st.Homepage, "")
	case 5:
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n%s\n\nLicenza: %s\nDisponibilità: %s\nStato: %s\n", st.Name, st.Description, st.License, st.Availability, st.Status)
		if st.ExePath != "" {
			fmt.Fprintf(&b, "\nFile: %s\nSHA-256: %s\n", st.ExePath, st.SHA256)
			if st.SealedHash != "" && st.SealedHash != st.SHA256 {
				fmt.Fprintf(&b, "Sigillo: %s\n", st.SealedHash)
			}
		}
		if st.Hint != "" {
			fmt.Fprintf(&b, "\nEsempio: %s\n", st.Hint)
		}
		if st.Warning != "" {
			fmt.Fprintf(&b, "\nAttenzione: %s\n", st.Warning)
		}
		if st.Alternative != "" {
			fmt.Fprintf(&b, "\n%s\n", st.Alternative)
		}
		fmt.Fprintf(&b, "\nFonte: %s", st.Homepage)
		messageBox(u.hwnd, b.String(), st.Name, mbIconInfo)
	}
}

func (u *ui) popup(m uintptr) int {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(u.hwnd)
	r, _, _ := pTrackPopupMenu.Call(m, tpmRetCmd|tpmRightBtn, uintptr(pt.X), uintptr(pt.Y), 0, u.hwnd, 0)
	return int(r)
}

func (u *ui) mainMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	defer pDestroyMenu.Call(m)
	add := func(id int, text string, checked, enabled bool) {
		f := uintptr(mfString)
		if checked {
			f |= mfChecked
		}
		if !enabled {
			f |= mfGrayed
		}
		pAppendMenuW.Call(m, f, uintptr(id), u16p(text))
	}
	sep := func() { pAppendMenuW.Call(m, mfSeparator, 0, 0) }
	add(20, "Aggiornamenti software…", false, true)
	add(21, "Strumenti personali…", false, true)
	sep()
	add(13, "Apri la cartella output", false, true)
	sep()
	add(2, "Sigilla il toolkit", false, !u.busy)
	add(3, "Verifica il toolkit", false, !u.busy)
	add(4, "Verifica il registro", false, true)
	add(5, "Apri il registro delle esecuzioni", false, true)
	add(6, "Apri la cartella di AFIP Portable", false, true)
	sep()
	add(7, "Sempre in primo piano", u.settings.Topmost, true)
	add(12, "Mostra solo gli strumenti presenti", u.settings.OnlyPresent, true)
	add(8, "Mostra anche gli strumenti solo Linux", u.settings.ShowUnavailable, true)
	add(9, "Ricarica il catalogo", false, true)
	sep()
	add(10, "Informazioni", false, true)
	add(11, "Esci", false, true)
	switch u.popup(m) {
	case 20:
		u.openUpdates()
	case 21:
		u.openCustom()
	case 13:
		shellOpen(u.hwnd, u.app.outputDir(), "")
	case 2:
		if messageBox(u.hwnd, "Sigillare il toolkit calcola l'SHA-256 di ogni file in tools\\ e runtime\\ e lo salva come riferimento.\n\nFarlo solo sulla propria postazione, dopo aver copiato o aggiornato gli strumenti. Continuare?", "Sigilla il toolkit", mbOKCancel|mbIconQuest) == idOK {
			u.runJob("Calcolo degli hash del toolkit…", func() func() {
				s, err := u.app.seal()
				return func() {
					if err != nil {
						messageBox(u.hwnd, err.Error(), "Sigillo non riuscito", mbIconError)
						return
					}
					u.say(fmt.Sprintf("Toolkit sigillato: %d file", s.Count), cOK)
					messageBox(u.hwnd, fmt.Sprintf("Toolkit sigillato.\n\nFile: %d\nImpronta: %s", s.Count, s.Digest), "Sigilla il toolkit", mbIconInfo)
				}
			})
		}
	case 3:
		u.runJob("Verifica del toolkit in corso…", func() func() {
			d, err := u.app.verify()
			return func() {
				switch {
				case err != nil:
					messageBox(u.hwnd, err.Error(), "Verifica non riuscita", mbIconError)
				case !d.Sealed:
					messageBox(u.hwnd, "Il toolkit non è ancora sigillato: non c'è un riferimento con cui confrontarlo.", "Verifica del toolkit", mbIconInfo)
				case d.Valid:
					u.say(fmt.Sprintf("Toolkit integro: %d file", d.Checked), cOK)
					messageBox(u.hwnd, fmt.Sprintf("Toolkit integro: %d file identici al sigillo.", d.Checked), "Verifica del toolkit", mbIconInfo)
				default:
					list := func(t string, xs []string) string {
						if len(xs) == 0 {
							return ""
						}
						if len(xs) > 12 {
							xs = append(xs[:12:12], fmt.Sprintf("… e altri %d", len(xs)-12))
						}
						return fmt.Sprintf("\n%s (%d):\n  %s\n", t, len(xs), strings.Join(xs, "\n  "))
					}
					u.say("Il toolkit è diverso dal sigillo", cBad)
					messageBox(u.hwnd, "Il toolkit è diverso dal sigillo."+list("Modificati", d.Changed)+list("Mancanti", d.Missing)+list("Aggiunti", d.Added), "Verifica del toolkit", mbIconWarn)
				}
			}
		})
	case 4:
		c := u.app.Ledger.Verify()
		u.logStat = c
		if c.Valid {
			messageBox(u.hwnd, fmt.Sprintf("Registro integro: %d voci.\n\nHash di testa:\n%s", c.Entries, c.Head), "Verifica del registro", mbIconInfo)
		} else {
			messageBox(u.hwnd, fmt.Sprintf("La catena del registro è interrotta alla voce %d.\nQualcuno ha modificato o cancellato righe di logs\\launch-log.jsonl fuori da AFIP Portable.", c.BrokenAt), "Verifica del registro", mbIconError)
		}
		u.redraw()
	case 5:
		shellOpen(u.hwnd, "notepad.exe", `"`+u.app.Ledger.path+`"`)
	case 6:
		shellOpen(u.hwnd, u.app.Base, "")
	case 7:
		u.setTopmost(!u.settings.Topmost)
	case 8:
		u.settings.ShowUnavailable = !u.settings.ShowUnavailable
		u.app.saveSettings(u.settings)
		u.layout()
		u.redraw()
	case 12:
		u.settings.OnlyPresent = !u.settings.OnlyPresent
		u.app.saveSettings(u.settings)
		u.layout()
		u.redraw()
	case 9:
		u.reloadManifest()
		u.redraw()
	case 10:
		messageBox(u.hwnd, fmt.Sprintf("AFIP Portable %s\n\nCartella: %s\nPostazione: %s\nOperatore: %s\n\nSHA-256 del launcher:\n%s\n\nOgni avvio lascia tracce sul sistema (Prefetch, Amcache, BAM, cronologia USB): il registro serve a documentarle.",
			version, u.app.Base, u.app.Host, u.app.User, u.app.SelfHash), "Informazioni", mbIconInfo)
	case 11:
		pPostMessageW.Call(u.hwnd, wmClose, 0, 0)
	}
}

// runJob esegue un lavoro lungo fuori dal thread dell'interfaccia; done() torna sul thread UI.
func (u *ui) runJob(label string, work func() func()) {
	u.busy = true
	u.say(label, cAmber)
	go func() {
		done := work()
		u.mu.Lock()
		u.jobResult = done
		u.mu.Unlock()
		pPostMessageW.Call(u.hwnd, wmJobDone, 0, 0)
	}()
}

func (u *ui) setTopmost(on bool) {
	u.settings.Topmost = on
	after := ^uintptr(1) // HWND_NOTOPMOST (-2)
	if on {
		after = ^uintptr(0) // HWND_TOPMOST (-1)
	}
	pSetWindowPos.Call(u.hwnd, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	u.app.saveSettings(u.settings)
	u.redraw()
}

func (u *ui) setCompact(on bool) {
	var wr rect
	pGetWindowRect.Call(u.hwnd, uintptr(unsafe.Pointer(&wr)))
	if on {
		u.normalH = wr.B - wr.T
	}
	u.settings.Compact = on
	h := u.s(headerH)
	if !on {
		h = u.normalH
		if h < u.s(minH) {
			h = u.s(640)
		}
	}
	pSetWindowPos.Call(u.hwnd, 0, 0, 0, uintptr(wr.R-wr.L), uintptr(h), swpNoMove|swpNoZOrder|swpNoActivate)
	u.placeEdit()
	u.saveGeometry()
	u.redraw()
}

func (u *ui) saveGeometry() {
	var wr rect
	pGetWindowRect.Call(u.hwnd, uintptr(unsafe.Pointer(&wr)))
	u.settings.X, u.settings.Y, u.settings.W = int(wr.L), int(wr.T), int(wr.R-wr.L)
	if !u.settings.Compact {
		u.settings.H = int(wr.B - wr.T)
	} else if u.normalH > 0 {
		u.settings.H = int(u.normalH)
	}
	u.app.saveSettings(u.settings)
}

func (u *ui) moveSel(delta int) {
	i := u.sel
	for step := 0; step < len(u.rows); step++ {
		i += delta
		if i < 0 || i >= len(u.rows) {
			return
		}
		if !u.rows[i].header {
			break
		}
	}
	if i < 0 || i >= len(u.rows) || u.rows[i].header {
		return
	}
	u.setSel(i)
	lr := u.listRect()
	r := u.rows[i]
	if r.y < u.scroll {
		u.scroll = r.y
	} else if r.y+r.h > u.scroll+(lr.B-lr.T) {
		u.scroll = r.y + r.h - (lr.B - lr.T)
	}
	u.clampScroll()
	u.redraw()
}

// handleKey: tastiera comune alla finestra e al campo di ricerca. true = gestito.
func (u *ui) handleKey(vk uintptr) bool {
	switch vk {
	case vkDown:
		u.moveSel(1)
	case vkUp:
		u.moveSel(-1)
	case vkNext:
		for i := 0; i < 8; i++ {
			u.moveSel(1)
		}
	case vkPrior:
		for i := 0; i < 8; i++ {
			u.moveSel(-1)
		}
	case vkReturn:
		if u.sel >= 0 {
			u.activate(u.sel, "")
		}
	case vkEscape:
		if getText(u.edit) != "" {
			pSetWindowTextW.Call(u.edit, u16p(""))
		} else {
			return false
		}
	default:
		return false
	}
	return true
}

// --- procedura della finestra -----------------------------------------------------------------
func (u *ui) wndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch m {
	case wmNcCalcSize:
		if wp != 0 {
			return 0 // tutta la finestra e' area client: bordi e titolo li disegniamo noi
		}
	case wmNcHitTest:
		pt := point{loword(lp), hiword(lp)}
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		c := u.client()
		e := u.s(6)
		left, right, top, bottom := pt.X < e, pt.X >= c.R-e, pt.Y < e/2, pt.Y >= c.B-e
		if u.settings.Compact {
			top, bottom = false, false
		}
		switch {
		case top && left:
			return htTopLeft
		case top && right:
			return htTopRight
		case bottom && left:
			return htBottomLeft
		case bottom && right:
			return htBottomRight
		case left:
			return htLeft
		case right:
			return htRight
		case top:
			return htTop
		case bottom:
			return htBottom
		}
		if pt.Y < u.s(headerH) && u.btnAt(pt.X, pt.Y) < 0 {
			return htCaption
		}
		return htClient
	case wmGetMinMaxInfo:
		mm := (*minMaxInfo)(unsafe.Pointer(lp))
		mm.minTrack = point{u.s(minW), u.s(minH)}
		if u.settings.Compact {
			mm.minTrack.Y, mm.maxTrack.Y = u.s(headerH), u.s(headerH)
		}
		return 0
	case wmSize:
		if u.edit != 0 {
			u.placeEdit()
			u.layout()
			u.redraw()
		}
		return 0
	case wmExitSizeMove:
		u.saveGeometry()
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		u.onPaint()
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cText)
		pSetBkColor.Call(wp, cInk)
		return u.brush(cInk)
	case wmCommand:
		if loword(wp) == editID && hiword(wp) == enChange {
			u.scroll = 0
			u.selID = ""
			u.layout()
			u.redraw()
		}
		return 0
	case wmMouseMove:
		x, y := loword(lp), hiword(lp)
		hb, hr, hc := u.btnAt(x, y), u.rowAt(x, y), false
		if hb != u.hoverBtn || hr != u.hoverRow || hc != u.hoverCase {
			u.hoverBtn, u.hoverRow, u.hoverCase = hb, hr, hc
			u.redraw()
		}
		if !u.tracking {
			tme := trackMouseEvent{cbSize: uint32(unsafe.Sizeof(trackMouseEvent{})), dwFlags: tmeLeave, hwndTrack: hwnd}
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			u.tracking = true
		}
		return 0
	case wmMouseLeave:
		u.tracking = false
		u.hoverBtn, u.hoverRow, u.hoverCase = -1, -1, false
		u.redraw()
		return 0
	case wmSetCursor:
		if loword(lp) == htClient && (u.hoverBtn >= 0 || u.hoverRow >= 0 || u.hoverCase) {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonDown:
		x, y := loword(lp), hiword(lp)
		u.downAt = u.target(x, y)
		pSetFocus.Call(hwnd)
		return 0
	case wmLButtonUp:
		x, y := loword(lp), hiword(lp)
		t := u.target(x, y)
		if t == "" || t != u.downAt {
			return 0
		}
		u.downAt = ""
		switch {
		case strings.HasPrefix(t, "btn:"):
			switch t {
			case "btn:0":
				pPostMessageW.Call(hwnd, wmClose, 0, 0)
			case "btn:1":
				u.setCompact(!u.settings.Compact)
			case "btn:2":
				u.setTopmost(!u.settings.Topmost)
			case "btn:3":
				u.mainMenu()
			}
		case strings.HasPrefix(t, "row:"):
			i, _ := strconv.Atoi(t[4:])
			if !u.rows[i].header {
				u.setSel(i)
			}
			u.activate(i, "")
		}
		return 0
	case wmRButtonUp:
		x, y := loword(lp), hiword(lp)
		if i := u.rowAt(x, y); i >= 0 && !u.rows[i].header {
			u.setSel(i)
			u.redraw()
			u.toolMenu(i)
		} else if y < u.s(headerH) {
			u.mainMenu()
		}
		return 0
	case wmMouseWheel:
		u.scroll -= hiword(wp) * u.s(toolRowH) * 3 / 120
		u.clampScroll()
		u.redraw()
		return 0
	case wmKeyDown:
		if wp == 'F' && isCtrl() {
			pSetFocus.Call(u.edit)
			pSendMessageW.Call(u.edit, emSetSel, 0, ^uintptr(0))
			return 0
		}
		if u.handleKey(wp) {
			return 0
		}
	case wmChar:
		if wp >= 0x20 && wp != 0x7f { // digitare nella finestra cerca direttamente
			pSetFocus.Call(u.edit)
			pSendMessageW.Call(u.edit, wmChar, wp, lp)
			return 0
		}
	case wmStates:
		u.mu.Lock()
		st, lg := u.pending, u.pendLog
		u.pending = nil
		u.mu.Unlock()
		if st != nil {
			u.states, u.logStat, u.hashed = st, lg, true
			u.layout()
			u.redraw()
		}
		return 0
	case wmRunUI:
		u.drainQueue()
		return 0
	case wmJobDone:
		u.mu.Lock()
		done := u.jobResult
		u.jobResult = nil
		u.mu.Unlock()
		u.busy = false
		if done != nil {
			done()
		}
		u.refreshAsync()
		return 0
	case wmDpiChanged:
		u.dpi = loword(wp)
		u.makeFonts()
		for k, v := range u.pens {
			pDeleteObject.Call(v)
			delete(u.pens, k)
		}
		r := (*rect)(unsafe.Pointer(lp))
		pSetWindowPos.Call(hwnd, 0, uintptr(r.L), uintptr(r.T), uintptr(r.R-r.L), uintptr(r.B-r.T), swpNoZOrder|swpNoActivate)
		return 0
	case wmQueryEndSess:
		return 1
	case wmEndSession:
		if wp != 0 {
			u.saveGeometry()
			u.app.log(LogEntry{Action: "session.end", Detail: "fine sessione di Windows"})
		}
		return 0
	case wmClose:
		u.saveGeometry()
		u.app.log(LogEntry{Action: "session.end", Detail: "finestra chiusa"})
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// missingDownloads: vero se nessuno degli strumenti scaricabili da GitHub e' presente.
func (u *ui) missingDownloads() bool {
	n := 0
	for _, st := range u.states {
		if st.Fetch["github"] != nil {
			n++
			if st.ExePath != "" {
				return false
			}
		}
	}
	return n > 0
}

func isCtrl() bool {
	s, _, _ := pGetKeyState.Call(vkCtrl)
	return s&0x8000 != 0
}

func (u *ui) target(x, y int32) string {
	if b := u.btnAt(x, y); b >= 0 {
		return "btn:" + strconv.Itoa(b)
	}
	if u.settings.Compact {
		return ""
	}
	if i := u.rowAt(x, y); i >= 0 {
		return "row:" + strconv.Itoa(i)
	}
	return ""
}

// Il campo di ricerca inoltra frecce, Invio ed Esc all'elenco.
func (u *ui) editProc(hwnd, m, wp, lp uintptr) uintptr {
	switch m {
	case wmKeyDown:
		if wp == 'A' && isCtrl() { // seleziona tutto: non tutte le versioni del controllo EDIT lo fanno
			pSendMessageW.Call(hwnd, emSetSel, 0, ^uintptr(0))
			return 0
		}
		if wp == 'F' && isCtrl() {
			pSendMessageW.Call(hwnd, emSetSel, 0, ^uintptr(0))
			return 0
		}
		if u.handleKey(wp) {
			return 0
		}
	case wmChar:
		if wp == vkReturn || wp == vkEscape || wp == 0x01 || wp == 0x06 {
			return 0 // evita il "bip" di Windows (Invio, Esc, Ctrl+A, Ctrl+F)
		}
	}
	r, _, _ := pCallWindowProcW.Call(u.editOrig, hwnd, m, wp, lp)
	return r
}

var appIconBig, appIconSmall uintptr

func loadAppIcon() {
	exe, _ := os.Executable()
	pExtractIconExW.Call(u16p(exe), 0, uintptr(unsafe.Pointer(&appIconBig)), uintptr(unsafe.Pointer(&appIconSmall)), 1)
}

func setDarkFrame(hwnd uintptr) {
	if pDwmSetWindowAttribute.Find() != nil {
		return
	}
	on := int32(1)
	pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&on)), 4) // DWMWA_USE_IMMERSIVE_DARK_MODE
	round := int32(2)
	pDwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&round)), 4) // DWMWA_WINDOW_CORNER_PREFERENCE: arrotondati (Windows 11)
}

func runGUI(a *App) {
	if pSetProcessDpiAwarenessCtx.Find() == nil {
		pSetProcessDpiAwarenessCtx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	} else if pSetProcessDPIAware.Find() == nil {
		pSetProcessDPIAware.Call()
	}
	// Una sola istanza per pendrive: se e' gia' aperta, la si porta in primo piano.
	pCreateMutexW.Call(0, 0, u16p("Local\\AFIP-Portable-"+shortHash(a.Base)))
	if err := syscall.GetLastError(); err == syscall.Errno(183) {
		if h, _, _ := pFindWindowW.Call(u16p(className), 0); h != 0 {
			pShowWindow.Call(h, swRestore)
			pSetForegroundWindow.Call(h)
		}
		return
	}
	a.log(LogEntry{Action: "session.start", Detail: fmt.Sprintf("AFIP Portable %s; base %s; sha256 launcher %s", version, a.Base, a.SelfHash)})
	loadAppIcon()
	u := &ui{app: a, dpi: 96, hoverBtn: -1, hoverRow: -1, sel: -1, brushes: map[uintptr]uintptr{}, pens: map[uintptr]uintptr{}}
	theUI = u
	u.settings = a.loadSettings()
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cur, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{style: 0x0008, lpfnWndProc: syscall.NewCallback(func(h, m, w, l uintptr) uintptr {
		if theUI.hwnd == 0 {
			theUI.hwnd = h
		}
		return theUI.wndProc(h, m, w, l)
	}), hInstance: hinst, hIcon: appIconBig, hCursor: cur, lpszClassName: u16(className), hIconSm: appIconSmall}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Posizione: quella salvata se ancora su uno schermo, altrimenti sul bordo destro dello schermo principale.
	w, h, x, y := int32(u.settings.W), int32(u.settings.H), int32(u.settings.X), int32(u.settings.Y)
	pt := point{x + 20, y + 10}
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 0)
	if w <= 0 || h <= 0 || mon == 0 {
		mon, _, _ = pMonitorFromPoint.Call(0, 1)
		mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		w, h = 320, 680
		if wh := mi.work.B - mi.work.T - 80; h > wh {
			h = wh
		}
		x, y = mi.work.R-w-24, mi.work.T+40
	}
	ex := uintptr(wsExAppWindow)
	if u.settings.Topmost {
		ex |= wsExTopmost
	}
	hwnd, _, _ := pCreateWindowExW.Call(ex, u16p(className), u16p("AFIP Portable"),
		wsPopup|wsThickFrame|wsSysMenu|wsMinimizeBox|wsClipChildren, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, hinst, 0)
	if hwnd == 0 {
		a.log(LogEntry{Action: "session.end", Detail: "impossibile creare la finestra"})
		messageBox(0, "Impossibile creare la finestra di AFIP Portable.", "AFIP Portable", mbIconError)
		return
	}
	u.hwnd = hwnd
	if pGetDpiForWindow.Find() == nil {
		if d, _, _ := pGetDpiForWindow.Call(hwnd); d > 0 {
			u.dpi = int32(d)
		}
	}
	if u.settings.W <= 0 && u.dpi != 96 { // prima apertura su schermo ad alta densita'
		pSetWindowPos.Call(hwnd, 0, uintptr(x-(u.s(320)-w)), uintptr(y), uintptr(u.s(320)), uintptr(u.s(680)), swpNoZOrder)
	}
	setDarkFrame(hwnd)
	u.makeFonts()
	u.edit, _, _ = pCreateWindowExW.Call(0, u16p("EDIT"), 0, wsChild|wsVisible|wsTabStop|esAutoHScroll, 0, 0, 10, 10, hwnd, editID, hinst, 0)
	pSendMessageW.Call(u.edit, wmSetFont, u.fBody, 1)
	pSendMessageW.Call(u.edit, emSetCueBanner, 1, u16p("Cerca uno strumento (Ctrl+F)"))
	u.editOrig, _, _ = pSetWindowLongPtrW.Call(u.edit, gwlpWndProc, syscall.NewCallback(func(h, m, w, l uintptr) uintptr { return theUI.editProc(h, m, w, l) }))
	u.reloadManifest()
	if u.settings.Compact {
		u.normalH = int32(u.settings.H)
		u.settings.Compact = false
		u.setCompact(true)
	}
	u.placeEdit()
	u.say(fmt.Sprintf("Postazione %s · %s · ogni avvio viene registrato", a.Host, a.User), cMuted)
	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)
	pSetFocus.Call(u.edit)
	// Pacchetto leggero: se nessuno strumento GitHub e' ancora presente, si propone di scaricarli subito.
	if u.missingDownloads() {
		if messageBox(hwnd, "Gli strumenti non sono ancora nella pendrive.\n\nAprire \"Aggiornamenti software\" per scaricarli dalle release ufficiali su GitHub? "+
			"Ogni file viene verificato con l'SHA-256 pubblicato e registrato. Servono internet e circa 2,5 GB liberi.", "AFIP Portable", mbYesNo|mbIconQuest) == idYes {
			u.openUpdates()
		}
	}

	var mm msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&mm)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if activeDialog != 0 {
			if d, _, _ := pIsDialogMessageW.Call(activeDialog, uintptr(unsafe.Pointer(&mm))); d != 0 {
				continue
			}
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&mm)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&mm)))
	}
}

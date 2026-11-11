//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// listWin: finestra secondaria con titolo, testo, elenco a colonne (con caselle di spunta facoltative) e pulsanti.
// Usata per "Aggiornamenti software" e "Strumenti personali". Tutto disegnato con lo stesso stile del pannello.
type lwCol struct {
	title string
	frac  float64 // larghezza relativa
}

type lwRow struct {
	id        string
	cols      []string
	colors    []uintptr // 0 = colore normale
	checkable bool
	checked   bool
}

type lwBtn struct {
	label   string
	primary bool
	enabled func() bool
	click   func()
}

type listWin struct {
	u        *ui
	hwnd     uintptr
	title    string
	info     string
	status   string
	stColor  uintptr
	cols     []lwCol
	rows     []lwRow
	buttons  []lwBtn
	sel      int
	scroll   int32
	hoverRow int
	hoverBtn int
	tracking bool
	onClose  func()
	canClose func() bool
}

const lwClass = "AFIPPortableList"

var (
	lwWindows    = map[uintptr]*listWin{}
	lwCreating   *listWin
	lwRegistered bool
)

func (u *ui) openListWin(lw *listWin, w, h int32) {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	if !lwRegistered {
		cur, _, _ := pLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{lpfnWndProc: syscall.NewCallback(lwProc), hInstance: hinst, hCursor: cur, lpszClassName: u16(lwClass), hIcon: appIconBig, hIconSm: appIconSmall}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		lwRegistered = true
	}
	lw.u, lw.sel, lw.hoverRow, lw.hoverBtn = u, -1, -1, -1
	// centrata sullo schermo del pannello, dentro l'area visibile
	mon, _, _ := pMonitorFromWindow.Call(u.hwnd, 2)
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	w, h = u.s(w), u.s(h)
	if mw := mi.work.R - mi.work.L - u.s(40); w > mw {
		w = mw
	}
	if mh := mi.work.B - mi.work.T - u.s(40); h > mh {
		h = mh
	}
	x := mi.work.L + (mi.work.R-mi.work.L-w)/2
	y := mi.work.T + (mi.work.B-mi.work.T-h)/2
	lwCreating = lw
	hwnd, _, _ := pCreateWindowExW.Call(wsExAppWindow, u16p(lwClass), u16p(lw.title), wsPopup|wsCaption|wsSysMenu|wsThickFrame|wsMinimizeBox|wsClipChildren,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), u.hwnd, 0, hinst, 0)
	lwCreating = nil
	lw.hwnd = hwnd
	lwWindows[hwnd] = lw
	setDarkFrame(hwnd)
	pShowWindow.Call(hwnd, swShow)
	pSetForegroundWindow.Call(hwnd)
}

func (lw *listWin) redraw() {
	if lw.hwnd != 0 {
		pInvalidateRect.Call(lw.hwnd, 0, 0)
	}
}

func (lw *listWin) client() rect {
	var r rect
	pGetClientRect.Call(lw.hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func (lw *listWin) layout() (head, list, foot rect) {
	c, u := lw.client(), lw.u
	head = rect{0, 0, c.R, u.s(78)}
	foot = rect{0, c.B - u.s(86), c.R, c.B}
	list = rect{0, head.B + u.s(28), c.R, foot.T}
	return
}

func (lw *listWin) rowH() int32 { return lw.u.s(32) }

func (lw *listWin) btnRects() []rect {
	_, _, foot := lw.layout()
	u := lw.u
	out := make([]rect, len(lw.buttons))
	x := foot.R - u.s(16)
	for i := len(lw.buttons) - 1; i >= 0; i-- {
		w := u.s(int32(36 + 7*len([]rune(lw.buttons[i].label))))
		out[i] = rect{x - w, foot.T + u.s(40), x, foot.T + u.s(72)}
		x -= w + u.s(8)
	}
	return out
}

func (lw *listWin) clamp() {
	_, list, _ := lw.layout()
	max := int32(len(lw.rows))*lw.rowH() - (list.B - list.T)
	if max < 0 {
		max = 0
	}
	if lw.scroll > max {
		lw.scroll = max
	}
	if lw.scroll < 0 {
		lw.scroll = 0
	}
}

func (lw *listWin) rowAt(x, y int32) int {
	_, list, _ := lw.layout()
	if !inRect(list, x, y) {
		return -1
	}
	i := int((y - list.T + lw.scroll) / lw.rowH())
	if i < 0 || i >= len(lw.rows) {
		return -1
	}
	return i
}

func (lw *listWin) btnAt(x, y int32) int {
	for i, r := range lw.btnRects() {
		if inRect(r, x, y) {
			return i
		}
	}
	return -1
}

func (lw *listWin) colX(c rect) []int32 {
	u := lw.u
	x := u.s(16)
	if lw.anyCheckable() {
		x += u.s(28)
	}
	w := float64(c.R - x - u.s(16))
	xs := []int32{}
	for _, col := range lw.cols {
		xs = append(xs, x)
		x += int32(w * col.frac)
	}
	return append(xs, c.R-u.s(16))
}

func (lw *listWin) anyCheckable() bool {
	for _, r := range lw.rows {
		if r.checkable {
			return true
		}
	}
	return false
}

func (lw *listWin) paint(dc uintptr) {
	u := lw.u
	p := painter{u, dc}
	c := lw.client()
	pSetBkMode.Call(dc, 1)
	p.fill(c, cInk)
	head, list, foot := lw.layout()
	p.fill(head, cPanel)
	p.text(lw.title, rect{u.s(16), u.s(12), head.R - u.s(16), u.s(36)}, u.fTitle, cText, dtLeft|dtVCenter|dtSingle)
	p.text(lw.info, rect{u.s(16), u.s(38), head.R - u.s(16), head.B - u.s(6)}, u.fSmall, cMuted, dtLeft|dtWordBreak)
	p.fill(rect{0, head.B - 1, c.R, head.B}, cLine)
	// intestazioni delle colonne
	xs := lw.colX(c)
	for i, col := range lw.cols {
		p.text(col.title, rect{xs[i], head.B + u.s(4), xs[i+1] - u.s(8), list.T - u.s(2)}, u.fSmall, cMuted, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	}
	p.fill(rect{u.s(12), list.T - 1, c.R - u.s(12), list.T}, cLine)
	pIntersectClipRect.Call(dc, uintptr(list.L), uintptr(list.T), uintptr(list.R), uintptr(list.B))
	rh := lw.rowH()
	for i, r := range lw.rows {
		top := list.T + int32(i)*rh - lw.scroll
		if top+rh < list.T || top > list.B {
			continue
		}
		rr := rect{u.s(8), top + u.s(1), c.R - u.s(8), top + rh - u.s(1)}
		if i == lw.sel {
			p.round(rr, u.s(6), cRaise, cAmber, true)
		} else if i == lw.hoverRow {
			p.round(rr, u.s(6), cRaise, 0, false)
		}
		if r.checkable {
			cb := rect{u.s(18), top + rh/2 - u.s(8), u.s(34), top + rh/2 + u.s(8)}
			if r.checked {
				p.round(cb, u.s(4), cAmber, cAmber, true)
				p.line(cb.L+u.s(4), cb.T+u.s(8), cb.L+u.s(7), cb.T+u.s(11), cOnAmbr, u.s(2))
				p.line(cb.L+u.s(7), cb.T+u.s(11), cb.L+u.s(12), cb.T+u.s(5), cOnAmbr, u.s(2))
			} else {
				p.round(cb, u.s(4), cInk, cMuted, true)
			}
		}
		for j, txt := range r.cols {
			if j >= len(lw.cols) {
				break
			}
			col, font := cText, u.fBody
			if j == 0 {
				font = u.fName
			}
			if j < len(r.colors) && r.colors[j] != 0 {
				col = r.colors[j]
			}
			p.text(txt, rect{xs[j], top, xs[j+1] - u.s(8), top + rh}, font, col, dtLeft|dtVCenter|dtSingle|dtEllipsis)
		}
	}
	if len(lw.rows) == 0 {
		p.text("Nessun elemento.", rect{u.s(16), list.T + u.s(12), list.R - u.s(16), list.T + u.s(40)}, u.fBody, cMuted, dtLeft|dtSingle)
	}
	if vis := list.B - list.T; int32(len(lw.rows))*rh > vis {
		total := int32(len(lw.rows)) * rh
		th := vis * vis / total
		if th < u.s(24) {
			th = u.s(24)
		}
		ty := list.T + (vis-th)*lw.scroll/(total-vis)
		p.round(rect{c.R - u.s(6), ty, c.R - u.s(2), ty + th}, u.s(4), cLine, 0, false)
	}
	pSelectClipRgn.Call(dc, 0)
	p.fill(foot, cPanel)
	p.fill(rect{0, foot.T, c.R, foot.T + 1}, cLine)
	br := lw.btnRects()
	stR := rect{u.s(16), foot.T + u.s(6), c.R - u.s(16), foot.T + u.s(34)} // riga di stato, sopra i pulsanti
	sc := lw.stColor
	if sc == 0 {
		sc = cMuted
	}
	p.text(lw.status, stR, u.fBody, sc, dtLeft|dtVCenter|dtSingle|dtEllipsis)
	for i, b := range lw.buttons {
		en := b.enabled == nil || b.enabled()
		fill, txt, border := cRaise, cText, cLine
		if b.primary {
			fill, txt, border = cAmber, cOnAmbr, cAmber
		}
		if !en {
			fill, txt, border = cPanel, cLine, cLine
		} else if i == lw.hoverBtn {
			if b.primary {
				fill = rgb(0xe6, 0xb4, 0x55)
			} else {
				fill = cLine
			}
		}
		p.round(br[i], u.s(6), fill, border, true)
		p.text(b.label, br[i], u.fName, txt, dtCenter|dtVCenter|dtSingle)
	}
}

func lwProc(hwnd, m, wp, lp uintptr) uintptr {
	lw := lwWindows[hwnd]
	if lw == nil {
		lw = lwCreating
	}
	if lw == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	if lw.hwnd == 0 {
		lw.hwnd = hwnd
	}
	u := lw.u
	switch m {
	case wmGetMinMaxInfo:
		mm := (*minMaxInfo)(unsafe.Pointer(lp))
		mm.minTrack = point{u.s(520), u.s(320)}
		return 0
	case wmSize:
		lw.clamp()
		lw.redraw()
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		c := lw.client()
		mem, _, _ := pCreateCompatibleDC.Call(hdc)
		bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(c.R), uintptr(c.B))
		old, _, _ := pSelectObject.Call(mem, bmp)
		lw.paint(mem)
		pBitBlt.Call(hdc, 0, 0, uintptr(c.R), uintptr(c.B), mem, 0, 0, srcCopy)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mem)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmMouseMove:
		x, y := loword(lp), hiword(lp)
		hr, hb := lw.rowAt(x, y), lw.btnAt(x, y)
		if hr != lw.hoverRow || hb != lw.hoverBtn {
			lw.hoverRow, lw.hoverBtn = hr, hb
			lw.redraw()
		}
		if !lw.tracking {
			tme := trackMouseEvent{cbSize: uint32(unsafe.Sizeof(trackMouseEvent{})), dwFlags: tmeLeave, hwndTrack: hwnd}
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			lw.tracking = true
		}
		return 0
	case wmMouseLeave:
		lw.tracking, lw.hoverRow, lw.hoverBtn = false, -1, -1
		lw.redraw()
		return 0
	case wmSetCursor:
		if loword(lp) == htClient && (lw.hoverBtn >= 0 || lw.hoverRow >= 0) {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonUp:
		x, y := loword(lp), hiword(lp)
		if b := lw.btnAt(x, y); b >= 0 {
			if lw.buttons[b].enabled == nil || lw.buttons[b].enabled() {
				lw.buttons[b].click()
				lw.redraw()
			}
			return 0
		}
		if i := lw.rowAt(x, y); i >= 0 {
			lw.sel = i
			if lw.rows[i].checkable {
				lw.rows[i].checked = !lw.rows[i].checked
			}
			lw.redraw()
		}
		return 0
	case wmMouseWheel:
		lw.scroll -= hiword(wp) * lw.rowH() * 3 / 120
		lw.clamp()
		lw.redraw()
		return 0
	case wmKeyDown:
		if wp == vkEscape {
			pPostMessageW.Call(hwnd, wmClose, 0, 0)
			return 0
		}
	case wmClose:
		if lw.canClose != nil && !lw.canClose() {
			return 0
		}
		if lw.onClose != nil {
			lw.onClose()
		}
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		delete(lwWindows, hwnd)
		lw.hwnd = 0
		pSetForegroundWindow.Call(u.hwnd)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

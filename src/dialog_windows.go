//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

// Finestra modale con un campo obbligatorio: nome del caso, riferimento dell'autorizzazione, motivazione.
type inputDlg struct {
	hwnd, prompt, edit, errLbl, okBtn, cancelBtn uintptr
	editOrig                                     uintptr
	minLen                                       int
	result                                       string
	done, ok                                     bool
}

var (
	curDlg        *inputDlg
	activeDialog  uintptr
	dlgRegistered bool
)

const dlgClass = "AFIPPortableInput"

func (u *ui) inputBox(title, prompt, initial string, minLen int) (string, bool) {
	return u.inputBoxOwner(u.hwnd, title, prompt, initial, minLen)
}

// inputBoxOwner: come inputBox, ma modale rispetto a un'altra finestra (es. Strumenti personali).
func (u *ui) inputBoxOwner(owner uintptr, title, prompt, initial string, minLen int) (string, bool) {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	if !dlgRegistered {
		cur, _, _ := pLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{lpfnWndProc: syscall.NewCallback(dlgProc), hInstance: hinst, hCursor: cur, lpszClassName: u16(dlgClass), hIcon: appIconBig, hIconSm: appIconSmall}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		dlgRegistered = true
	}
	// Altezza del testo calcolata con il font reale, cosi' il messaggio non viene tagliato.
	w := u.s(440)
	pad := u.s(16)
	tr := rect{0, 0, w - 2*pad, 0}
	dc, _, _ := pGetDC.Call(owner)
	pSelectObject.Call(dc, u.fBody)
	ws, _ := syscall.UTF16FromString(strings.ReplaceAll(prompt, "\x00", ""))
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(&ws[0])), uintptr(len(ws)-1), uintptr(unsafe.Pointer(&tr)), dtWordBreak|dtCalcRect|dtNoPrefix)
	pReleaseDC.Call(owner, dc)
	promptH := tr.B + u.s(4)
	clientH := pad + promptH + u.s(10) + u.s(28) + u.s(6) + u.s(18) + u.s(10) + u.s(32) + pad
	var wr rect
	pGetWindowRect.Call(owner, uintptr(unsafe.Pointer(&wr)))
	ww, wh := w+u.s(16), clientH+u.s(40) // bordi e barra del titolo di sistema
	// Centrata sul pannello ma sempre dentro l'area visibile del monitor che lo contiene.
	mon, _, _ := pMonitorFromWindow.Call(owner, 2)
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	x, y := (wr.L+wr.R)/2-ww/2, wr.T+u.s(80)
	if x+ww > mi.work.R-u.s(12) {
		x = mi.work.R - u.s(12) - ww
	}
	if x < mi.work.L+u.s(12) {
		x = mi.work.L + u.s(12)
	}
	if y+wh > mi.work.B {
		y = mi.work.B - wh
	}

	d := &inputDlg{minLen: minLen}
	curDlg = d
	d.hwnd, _, _ = pCreateWindowExW.Call(wsExDlgModal|wsExTopmost, u16p(dlgClass), u16p(title), wsPopup|wsCaption|wsSysMenu,
		uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), owner, 0, hinst, 0)
	setDarkFrame(d.hwnd)
	var cr rect
	pGetClientRect.Call(d.hwnd, uintptr(unsafe.Pointer(&cr)))
	cw := cr.R - 2*pad
	yy := pad
	child := func(class, text string, style uintptr, x, y, w, h int32, id uintptr) uintptr {
		hw, _, _ := pCreateWindowExW.Call(0, u16p(class), u16p(text), wsChild|wsVisible|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), d.hwnd, id, hinst, 0)
		pSendMessageW.Call(hw, wmSetFont, u.fBody, 1)
		return hw
	}
	d.prompt = child("STATIC", prompt, 0, pad, yy, cw, promptH, 0)
	yy += promptH + u.s(10)
	d.edit = child("EDIT", initial, wsTabStop|esAutoHScroll|0x00800000, pad, yy, cw, u.s(28), 100) // WS_BORDER
	yy += u.s(28) + u.s(6)
	d.errLbl = child("STATIC", "", 0, pad, yy, cw, u.s(18), 0)
	yy += u.s(18) + u.s(10)
	bw := u.s(110)
	d.cancelBtn = child("BUTTON", "Annulla", wsTabStop|bsOwnerDraw, cr.R-pad-2*bw-u.s(8), yy, bw, u.s(32), idCancel)
	d.okBtn = child("BUTTON", "Conferma", wsTabStop|bsOwnerDraw, cr.R-pad-bw, yy, bw, u.s(32), idOK)
	d.editOrig, _, _ = pSetWindowLongPtrW.Call(d.edit, gwlpWndProc, syscall.NewCallback(dlgEditProc))
	pSendMessageW.Call(d.edit, emSetSel, 0, ^uintptr(0))

	pEnableWindow.Call(owner, 0)
	pShowWindow.Call(d.hwnd, swShow)
	pSetFocus.Call(d.edit)
	prev := activeDialog
	activeDialog = d.hwnd
	var mm msg
	for !d.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&mm)), 0, 0, 0)
		if int32(r) <= 0 {
			pPostQuitMessage.Call(0)
			break
		}
		if ok, _, _ := pIsDialogMessageW.Call(d.hwnd, uintptr(unsafe.Pointer(&mm))); ok != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&mm)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&mm)))
	}
	activeDialog = prev
	pEnableWindow.Call(owner, 1)
	pDestroyWindow.Call(d.hwnd)
	pSetForegroundWindow.Call(owner)
	curDlg = nil
	return d.result, d.ok
}

func (d *inputDlg) confirm() {
	v := strings.TrimSpace(getText(d.edit))
	if len([]rune(v)) < d.minLen {
		pSetWindowTextW.Call(d.errLbl, u16p("Campo obbligatorio: scrivi almeno "+itoa(d.minLen)+" caratteri."))
		pSetFocus.Call(d.edit)
		return
	}
	d.result, d.ok, d.done = v, true, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func dlgProc(hwnd, m, wp, lp uintptr) uintptr {
	d, u := curDlg, theUI
	if d == nil || u == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	switch m {
	case wmCommand:
		switch loword(wp) {
		case idOK:
			d.confirm()
		case idCancel:
			d.done = true
		}
		return 0
	case wmClose:
		d.done = true
		return 0
	case wmEraseBkgnd:
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		pFillRect.Call(wp, uintptr(unsafe.Pointer(&r)), u.brush(cPanel))
		return 1
	case wmCtlColorStatic:
		col := cText
		if lp == d.errLbl {
			col = cBad
		}
		pSetTextColor.Call(wp, col)
		pSetBkColor.Call(wp, cPanel)
		return u.brush(cPanel)
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cText)
		pSetBkColor.Call(wp, cInk)
		return u.brush(cInk)
	case wmDrawItem:
		di := (*drawItemStruct)(unsafe.Pointer(lp))
		p := painter{u, di.hDC}
		pSetBkMode.Call(di.hDC, 1)
		p.fill(di.rcItem, cPanel)
		fill, txt, border := cRaise, cText, cLine
		if di.ctlID == idOK {
			fill, txt, border = cAmber, cOnAmbr, cAmber
		}
		if di.itemState&odsSelected != 0 {
			fill = cLine
		}
		p.round(di.rcItem, u.s(6), fill, border, true)
		if di.itemState&odsFocus != 0 {
			fr := di.rcItem
			fr.L, fr.T, fr.R, fr.B = fr.L+u.s(3), fr.T+u.s(3), fr.R-u.s(3), fr.B-u.s(3)
			nb, _, _ := pGetStockObject.Call(nullBrsh)
			pSelectObject.Call(di.hDC, nb)
			pSelectObject.Call(di.hDC, u.pen(cText))
			pRoundRect.Call(di.hDC, uintptr(fr.L), uintptr(fr.T), uintptr(fr.R), uintptr(fr.B), uintptr(u.s(4)), uintptr(u.s(4)))
		}
		label := "Annulla"
		if di.ctlID == idOK {
			label = "Conferma"
		}
		p.text(label, di.rcItem, u.fName, txt, dtCenter|dtVCenter|dtSingle)
		return 1
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func dlgEditProc(hwnd, m, wp, lp uintptr) uintptr {
	d := curDlg
	if d == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	if m == wmKeyDown && wp == vkReturn {
		d.confirm()
		return 0
	}
	if m == wmKeyDown && wp == vkEscape {
		d.done = true
		return 0
	}
	if m == wmChar && (wp == vkReturn || wp == vkEscape) {
		return 0
	}
	r, _, _ := pCallWindowProcW.Call(d.editOrig, hwnd, m, wp, lp)
	return r
}

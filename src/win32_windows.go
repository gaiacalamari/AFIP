//go:build windows

package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW           = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pUpdateWindow               = user32.NewProc("UpdateWindow")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pBeginPaint                 = user32.NewProc("BeginPaint")
	pEndPaint                   = user32.NewProc("EndPaint")
	pGetClientRect              = user32.NewProc("GetClientRect")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pMoveWindow                 = user32.NewProc("MoveWindow")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pSetCursor                  = user32.NewProc("SetCursor")
	pMessageBoxW                = user32.NewProc("MessageBoxW")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pClientToScreen             = user32.NewProc("ClientToScreen")
	pScreenToClient             = user32.NewProc("ScreenToClient")
	pPostMessageW               = user32.NewProc("PostMessageW")
	pSendMessageW               = user32.NewProc("SendMessageW")
	pSetWindowTextW             = user32.NewProc("SetWindowTextW")
	pGetWindowTextW             = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW       = user32.NewProc("GetWindowTextLengthW")
	pSetFocus                   = user32.NewProc("SetFocus")
	pGetFocus                   = user32.NewProc("GetFocus")
	pDestroyWindow              = user32.NewProc("DestroyWindow")
	pEnableWindow               = user32.NewProc("EnableWindow")
	pGetDpiForWindow            = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiAwarenessCtx  = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	pMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	pMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	pTrackMouseEvent            = user32.NewProc("TrackMouseEvent")
	pSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	pCallWindowProcW            = user32.NewProc("CallWindowProcW")
	pFindWindowW                = user32.NewProc("FindWindowW")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pIsIconic                   = user32.NewProc("IsIconic")
	pFillRect                   = user32.NewProc("FillRect")
	pDrawTextW                  = user32.NewProc("DrawTextW")
	pDrawIconEx                 = user32.NewProc("DrawIconEx")
	pSetTimer                   = user32.NewProc("SetTimer")
	pKillTimer                  = user32.NewProc("KillTimer")
	pGetKeyState                = user32.NewProc("GetKeyState")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject               = gdi32.NewProc("SelectObject")
	pBitBlt                     = gdi32.NewProc("BitBlt")
	pDeleteObject               = gdi32.NewProc("DeleteObject")
	pDeleteDC                   = gdi32.NewProc("DeleteDC")
	pCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	pCreatePen                  = gdi32.NewProc("CreatePen")
	pCreateFontW                = gdi32.NewProc("CreateFontW")
	pSetTextColor               = gdi32.NewProc("SetTextColor")
	pSetBkMode                  = gdi32.NewProc("SetBkMode")
	pSetBkColor                 = gdi32.NewProc("SetBkColor")
	pRoundRect                  = gdi32.NewProc("RoundRect")
	pEllipse                    = gdi32.NewProc("Ellipse")
	pMoveToEx                   = gdi32.NewProc("MoveToEx")
	pLineTo                     = gdi32.NewProc("LineTo")
	pPolygon                    = gdi32.NewProc("Polygon")
	pGetStockObject             = gdi32.NewProc("GetStockObject")
	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW               = kernel32.NewProc("CreateMutexW")
	pAttachConsole              = kernel32.NewProc("AttachConsole")
	pExtractIconExW             = shell32.NewProc("ExtractIconExW")
	pShellExecuteW              = shell32.NewProc("ShellExecuteW")
	pDwmSetWindowAttribute      = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wsPopup        = 0x80000000
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipChildren = 0x02000000
	wsThickFrame   = 0x00040000
	wsSysMenu      = 0x00080000
	wsMinimizeBox  = 0x00020000
	wsTabStop      = 0x00010000
	wsCaption      = 0x00C00000
	wsExTopmost    = 0x00000008
	wsExAppWindow  = 0x00040000
	wsExDlgModal   = 0x00000001
	wsExLayered    = 0x00080000
	esAutoHScroll  = 0x0080
	bsOwnerDraw    = 0x000B

	wmCreate         = 0x0001
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmActivate       = 0x0006
	wmPaint          = 0x000F
	wmClose          = 0x0010
	wmQueryEndSess   = 0x0011
	wmEndSession     = 0x0016
	wmEraseBkgnd     = 0x0014
	wmSetCursor      = 0x0020
	wmGetMinMaxInfo  = 0x0024
	wmSetFont        = 0x0030
	wmDrawItem       = 0x002B
	wmNcCalcSize     = 0x0083
	wmNcHitTest      = 0x0084
	wmKeyDown        = 0x0100
	wmChar           = 0x0102
	wmCommand        = 0x0111
	wmTimer          = 0x0113
	wmCtlColorEdit   = 0x0133
	wmCtlColorStatic = 0x0138
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmMouseWheel     = 0x020A
	wmExitSizeMove   = 0x0232
	wmMouseLeave     = 0x02A3
	wmDpiChanged     = 0x02E0
	wmApp            = 0x8000
	enChange         = 0x0300
	emSetCueBanner   = 0x1501
	emSetSel         = 0x00B1

	htClient      = 1
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	swShow       = 5
	swMinimize   = 6
	swRestore    = 9
	swShowNormal = 1

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpFrameChgd  = 0x0020

	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800
	tpmRetCmd   = 0x0100
	tpmRightBtn = 0x0002

	mbOK          = 0x00
	mbOKCancel    = 0x01
	mbYesNoCancel = 0x03
	mbYesNo       = 0x04
	mbIconError   = 0x10
	mbIconQuest   = 0x20
	mbIconWarn    = 0x30
	mbIconInfo    = 0x40
	idOK          = 1
	idCancel      = 2
	idYes         = 6
	idNo          = 7

	dtLeft      = 0x0000
	dtCenter    = 0x0001
	dtRight     = 0x0002
	dtVCenter   = 0x0004
	dtWordBreak = 0x0010
	dtSingle    = 0x0020
	dtCalcRect  = 0x0400
	dtNoPrefix  = 0x0800
	dtEllipsis  = 0x8000

	vkTab    = 0x09
	vkReturn = 0x0D
	vkEscape = 0x1B
	vkPrior  = 0x21
	vkNext   = 0x22
	vkUp     = 0x26
	vkDown   = 0x28
	vkCtrl   = 0x11

	idcArrow = 32512
	idcHand  = 32649
	tmeLeave = 0x00000002
	srcCopy  = 0x00CC0020
	nullPen  = 8
	nullBrsh = 5

	gwlpWndProc = ^uintptr(3) // -4
	odsSelected = 0x0001
	odsFocus    = 0x0010
	lwaAlpha    = 0x2
)

type point struct{ X, Y int32 }
type rect struct{ L, T, R, B int32 }

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type paintStruct struct {
	hdc       uintptr
	fErase    int32
	rcPaint   rect
	fRestore  int32
	fIncUpd   int32
	rgbReserv [32]byte
}

type trackMouseEvent struct {
	cbSize    uint32
	dwFlags   uint32
	hwndTrack uintptr
	hoverTime uint32
}

type minMaxInfo struct {
	reserved, maxSize, maxPos, minTrack, maxTrack point
}

type monitorInfo struct {
	cbSize  uint32
	monitor rect
	work    rect
	flags   uint32
}

type drawItemStruct struct {
	ctlType, ctlID, itemID, itemAction, itemState uint32
	hwndItem, hDC                                 uintptr
	rcItem                                        rect
	itemData                                      uintptr
}

func u16(s string) *uint16  { p, _ := syscall.UTF16PtrFromString(s); return p }
func u16p(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }

func loword(v uintptr) int32 { return int32(int16(v & 0xFFFF)) }
func hiword(v uintptr) int32 { return int32(int16((v >> 16) & 0xFFFF)) }

func rgb(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func messageBox(hwnd uintptr, text, title string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(hwnd, u16p(text), u16p(title), flags)
	return int(r)
}

func getText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func shellOpen(hwnd uintptr, file, params string) error {
	var p uintptr
	if params != "" {
		p = u16p(params)
	}
	r, _, _ := pShellExecuteW.Call(hwnd, u16p("open"), u16p(file), p, 0, swShowNormal)
	if r <= 32 {
		return syscall.Errno(r)
	}
	return nil
}

var (
	pIntersectClipRect = gdi32.NewProc("IntersectClipRect")
	pSelectClipRgn     = gdi32.NewProc("SelectClipRgn")
)

var (
	pGetDC     = user32.NewProc("GetDC")
	pReleaseDC = user32.NewProc("ReleaseDC")
)

var (
	comdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
)

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

// openFileDialog mostra la finestra standard di Windows per scegliere un programma.
func openFileDialog(owner uintptr, title string) (string, bool) {
	buf := make([]uint16, 32768)
	// il filtro di Windows e' una sequenza di stringhe separate da NUL: va codificato a mano
	// (syscall.StringToUTF16 rifiuta i NUL e manderebbe in crash il programma)
	filter := utf16.Encode([]rune("Programmi (*.exe, *.bat, *.cmd, *.html)\x00*.exe;*.bat;*.cmd;*.html;*.htm\x00Tutti i file\x00*.*\x00\x00"))
	ofn := openFileName{hwndOwner: owner, lpstrFilter: &filter[0], nFilterIndex: 1, lpstrFile: &buf[0], nMaxFile: uint32(len(buf)),
		lpstrTitle: u16(title), Flags: 0x00001000 | 0x00000800 | 0x00080000 | 0x00000008} // FILEMUSTEXIST|PATHMUSTEXIST|EXPLORER|NOCHANGEDIR
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

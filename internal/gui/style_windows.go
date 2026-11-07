package gui

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

var (
	canvasColor        = walk.RGB(13, 17, 23)
	panelColor         = walk.RGB(22, 27, 34)
	panelBorderColor   = walk.RGB(48, 54, 61)
	cardColor          = walk.RGB(28, 33, 40)
	cardSelectedColor  = walk.RGB(33, 38, 45)
	cardSelectedBorder = walk.RGB(88, 166, 255)
	textColor          = walk.RGB(240, 246, 252)
	mutedColor         = walk.RGB(139, 148, 158)
	accentColor        = walk.RGB(88, 166, 255)
	accentHoverColor   = walk.RGB(121, 192, 255)
	accentPressColor   = walk.RGB(31, 111, 235)
	successColor       = walk.RGB(63, 185, 80)
	warningColor       = walk.RGB(210, 153, 34)
	dangerColor        = walk.RGB(248, 81, 73)
	dangerBgHover      = walk.RGB(45, 20, 24)
	dangerBgSelected   = walk.RGB(60, 24, 30)
	comctl             = syscall.NewLazyDLL("comctl32.dll")
	setSubclass        = comctl.NewProc("SetWindowSubclass")
	defSubclass        = comctl.NewProc("DefSubclassProc")
	getSubclass        = comctl.NewProc("GetWindowSubclass")
	gdi                = syscall.NewLazyDLL("gdi32.dll")
	createBrush        = gdi.NewProc("CreateSolidBrush")
	createPen          = gdi.NewProc("CreatePen")
	user32             = syscall.NewLazyDLL("user32.dll")
	getText            = user32.NewProc("GetWindowTextW")
	fillRect           = user32.NewProc("FillRect")
	buttonProc         = syscall.NewCallback(drawButtons)
	buttonSizeProc     uintptr
	listProc           = syscall.NewCallback(drawListBackground)
	listInputProc      = syscall.NewCallback(listInput)
	listBrush, _, _    = createBrush.Call(uintptr(canvasColor))
	avatarFont         *walk.Font
	titleFont          *walk.Font
	regularFont        *walk.Font
)

type accountList struct{ ListBox }

func init() {
	buttonSizeProc = syscall.NewCallback(buttonIdealSize)
	avatarFont, _ = walk.NewFont("Segoe UI", 13, walk.FontBold)
	titleFont, _ = walk.NewFont("Segoe UI", 10, walk.FontBold)
	regularFont, _ = walk.NewFont("Segoe UI", 9, 0)
}

func (l accountList) Create(b *Builder) error {
	w, e := walk.NewListBoxWithStyle(b.Parent(), win.LBS_OWNERDRAWVARIABLE|win.LBS_HASSTRINGS)
	if e != nil {
		return e
	}
	if l.AssignTo != nil {
		*l.AssignTo = w
	}
	return b.InitWidget(l, w, func() error {
		style := win.GetWindowLong(w.Handle(), win.GWL_STYLE)
		win.SetWindowLong(w.Handle(), win.GWL_STYLE, style&^win.WS_BORDER)
		win.SetWindowLong(w.Handle(), win.GWL_EXSTYLE, win.GetWindowLong(w.Handle(), win.GWL_EXSTYLE)&^win.WS_EX_CLIENTEDGE)
		w.SetItemStyler(l.ItemStyler)
		_ = w.GraphicsEffects().Clear()
		setSubclass.Call(uintptr(w.Handle()), listInputProc, 4, 0)
		if e := w.SetModel(l.Model); e != nil {
			return e
		}
		if l.OnCurrentIndexChanged != nil {
			w.CurrentIndexChanged().Attach(l.OnCurrentIndexChanged)
		}
		return nil
	})
}

type accountStyler struct{ ui *nativeUI }

func (s accountStyler) ItemHeightDependsOnWidth() bool { return false }
func (s accountStyler) DefaultItemHeight() int {
	if s.ui.list != nil {
		return s.ui.list.IntFrom96DPI(76)
	}
	return 76
}
func (s accountStyler) ItemHeight(int, int) int { return s.DefaultItemHeight() }
func (s accountStyler) StyleItem(st *walk.ListItemStyle) {
	i := st.Index()
	if i < 0 || i >= len(s.ui.accounts) || st.Canvas() == nil {
		return
	}
	px := s.ui.list.IntFrom96DPI
	c := st.Canvas()
	fill := func(color walk.Color, bounds walk.Rectangle, radius int) {
		b, err := walk.NewSolidColorBrush(color)
		if err != nil {
			return
		}
		defer b.Dispose()
		_ = c.FillRoundedRectanglePixels(b, bounds, walk.Size{Width: px(radius), Height: px(radius)})
	}
	stroke := func(color walk.Color, bounds walk.Rectangle, radius int) {
		p, err := walk.NewCosmeticPen(walk.PenSolid, color)
		if err != nil {
			return
		}
		defer p.Dispose()
		_ = c.DrawRoundedRectanglePixels(p, bounds, walk.Size{Width: px(radius), Height: px(radius)})
	}
	r := st.BoundsPixels()
	fill(canvasColor, r, 0)
	r.X += px(2)
	r.Width -= px(4)
	r.Height -= px(6)
	selected := i == s.ui.list.CurrentIndex()
	bg := panelColor
	border := panelBorderColor
	if selected {
		bg = cardSelectedColor
		border = cardSelectedBorder
	}
	fill(bg, r, 10)
	stroke(border, r, 10)
	if selected {
		fill(accentColor, walk.Rectangle{X: r.X, Y: r.Y + px(16), Width: px(3), Height: r.Height - px(32)}, 3)
	}
	a := s.ui.accounts[i]
	displayName := a.Email
	if a.Alias != "" {
		displayName = a.Alias
	}
	initial := "G"
	for _, ch := range displayName {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			initial = strings.ToUpper(string(ch))
			break
		}
	}
	badge := walk.Rectangle{X: r.X + px(16), Y: r.Y + px(16), Width: px(38), Height: px(38)}
	badgeBg := cardColor
	badgeBorder := panelBorderColor
	letterColor := accentColor
	if a.Email == s.ui.active {
		badgeBg = walk.RGB(20, 38, 28)
		badgeBorder = walk.RGB(35, 78, 52)
		letterColor = successColor
	}
	fill(badgeBg, badge, 8)
	stroke(badgeBorder, badge, 8)
	st.Font = avatarFont
	st.TextColor = letterColor
	st.DrawText(initial, badge, walk.TextSingleLine|walk.TextCenter|walk.TextVCenter|walk.TextNoPrefix)

	name := a.Email
	if a.Alias != "" {
		name = a.Alias
	}

	statusText := s.ui.t("Готов")
	statusBg := walk.RGB(23, 32, 45)
	statusBorder := walk.RGB(35, 56, 85)
	statusFg := accentColor
	if a.Email == s.ui.active {
		statusText = s.ui.t("активен")
		statusBg = walk.RGB(20, 38, 28)
		statusBorder = walk.RGB(35, 78, 52)
		statusFg = successColor
	} else if a.Paused {
		statusText = s.ui.t("пауза")
		statusBg = walk.RGB(36, 28, 46)
		statusBorder = walk.RGB(68, 50, 92)
		statusFg = walk.RGB(180, 145, 245)
	} else if a.Quota5H.RemainingFraction <= 0.05 && !a.Quota5H.ResetTime.IsZero() && time.Now().Before(a.Quota5H.ResetTime) {
		statusText = s.ui.t("кулдаун")
		statusBg = walk.RGB(40, 28, 16)
		statusBorder = walk.RGB(85, 58, 25)
		statusFg = warningColor
	}

	metricWidth := px(136)
	metricX := r.X + r.Width - px(20) - metricWidth*2 - px(24)
	nameRect := walk.Rectangle{X: r.X + px(68), Y: r.Y + px(14), Width: metricX - r.X - px(84), Height: px(22)}
	st.Font = titleFont
	st.TextColor = textColor
	st.DrawText(name, nameRect, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis|walk.TextNoPrefix)

	pillWidth := px(64)
	pillRect := walk.Rectangle{X: r.X + px(68), Y: r.Y + px(40), Width: pillWidth, Height: px(18)}
	fill(statusBg, pillRect, 4)
	stroke(statusBorder, pillRect, 4)
	st.Font = regularFont
	st.TextColor = statusFg
	st.DrawText(statusText, pillRect, walk.TextSingleLine|walk.TextCenter|walk.TextVCenter|walk.TextNoPrefix)

	emailRect := walk.Rectangle{X: pillRect.X + pillWidth + px(8), Y: r.Y + px(39), Width: metricX - (pillRect.X + pillWidth + px(16)), Height: px(20)}
	st.Font = regularFont
	st.TextColor = mutedColor
	st.DrawText(a.Email, emailRect, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis|walk.TextNoPrefix)

	for j, q := range []struct {
		label string
		value int
	}{
		{s.ui.t("5ч лимит"), percent(a.Quota5H.RemainingFraction)},
		{s.ui.t("Неделя"), percent(a.QuotaWeekly.RemainingFraction)},
	} {
		x := metricX + j*(metricWidth+px(24))
		st.Font = regularFont
		st.TextColor = mutedColor
		st.DrawText(q.label, walk.Rectangle{X: x, Y: r.Y + px(16), Width: metricWidth - px(44), Height: px(20)}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)

		st.TextColor = quotaColor(q.value)
		st.DrawText(fmt.Sprintf("%d%%", q.value), walk.Rectangle{X: x + metricWidth - px(44), Y: r.Y + px(16), Width: px(44), Height: px(20)}, walk.TextSingleLine|walk.TextVCenter|walk.TextRight)

		bar := walk.Rectangle{X: x, Y: r.Y + px(44), Width: metricWidth, Height: px(5)}
		fill(canvasColor, bar, 4)
		bar.Width = metricWidth * q.value / 100
		if bar.Width > 0 {
			fill(quotaColor(q.value), bar, 4)
		}
	}
}

func drawListBackground(hwnd uintptr, msg uint32, wp, lp, id, data uintptr) uintptr {
	if msg == win.WM_CTLCOLORLISTBOX && lp == data {
		win.SetBkColor(win.HDC(wp), win.COLORREF(canvasColor))
		win.SetTextColor(win.HDC(wp), win.COLORREF(textColor))
		return listBrush
	}
	r, _, _ := defSubclass.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func listInput(hwnd uintptr, msg uint32, wp, lp, id, data uintptr) uintptr {
	if msg == win.WM_MOUSEMOVE && wp&win.MK_LBUTTON == 0 {
		return 0
	}
	r, _, _ := defSubclass.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

const buttonHot = 1 << 16

func buttonHovered(hwnd win.HWND) bool {
	var data uintptr
	getSubclass.Call(uintptr(hwnd), buttonSizeProc, 3, uintptr(unsafe.Pointer(&data)))
	return data&buttonHot != 0
}

func drawButtons(hwnd uintptr, msg uint32, wp, lp, id, data uintptr) uintptr {
	if msg == win.WM_DRAWITEM {
		d := (*win.DRAWITEMSTRUCT)(unsafe.Pointer(lp))
		if d != nil && d.CtlType == 4 {
			var t [512]uint16
			getText.Call(uintptr(d.HwndItem), uintptr(unsafe.Pointer(&t[0])), 512)
			textStr := syscall.UTF16ToString(t[:])
			isDanger := strings.Contains(textStr, "Удалить") || strings.Contains(textStr, "Delete") || strings.Contains(textStr, "Löschen") || strings.Contains(textStr, "Usuń") || strings.Contains(textStr, "Видалити")
			isPrimary := d.HwndItem == win.HWND(data)

			bg, fg, line := cardColor, textColor, panelBorderColor
			if isPrimary {
				bg = accentColor
				fg = walk.RGB(255, 255, 255)
				line = accentColor
			} else if isDanger {
				fg = mutedColor
				line = panelBorderColor
			}

			if d.ItemState&win.ODS_DISABLED != 0 {
				bg = panelColor
				fg = walk.RGB(85, 95, 112)
				line = cardColor
			} else if d.ItemState&win.ODS_SELECTED != 0 {
				if isPrimary {
					bg = accentPressColor
					line = bg
				} else if isDanger {
					bg = dangerBgSelected
					fg = dangerColor
					line = dangerColor
				} else {
					bg = walk.RGB(24, 28, 38)
					line = panelBorderColor
				}
			} else if buttonHovered(d.HwndItem) {
				if isPrimary {
					bg = accentHoverColor
					line = bg
				} else if isDanger {
					bg = dangerBgHover
					fg = dangerColor
					line = dangerColor
				} else {
					bg = walk.RGB(42, 49, 64)
					line = walk.RGB(68, 80, 100)
				}
			}

			hb, _, _ := createBrush.Call(uintptr(bg))
			hp, _, _ := createPen.Call(0, 1, uintptr(line))
			ob := win.SelectObject(d.HDC, win.HGDIOBJ(hb))
			op := win.SelectObject(d.HDC, win.HGDIOBJ(hp))
			r := d.RcItem
			background := win.SendMessage(win.GetParent(d.HwndItem), win.WM_CTLCOLORSTATIC, uintptr(d.HDC), uintptr(d.HwndItem))
			if background == 0 {
				background = hb
			}
			fillRect.Call(uintptr(d.HDC), uintptr(unsafe.Pointer(&r)), background)
			win.SelectObject(d.HDC, win.HGDIOBJ(hb))
			win.SelectObject(d.HDC, win.HGDIOBJ(hp))
			win.RoundRect(d.HDC, r.Left, r.Top, r.Right, r.Bottom, 8, 8)
			win.SelectObject(d.HDC, ob)
			win.SelectObject(d.HDC, op)
			win.DeleteObject(win.HGDIOBJ(hb))
			win.DeleteObject(win.HGDIOBJ(hp))
			f := win.SendMessage(d.HwndItem, win.WM_GETFONT, 0, 0)
			of := win.SelectObject(d.HDC, win.HGDIOBJ(f))
			win.SetBkMode(d.HDC, 1)
			win.SetTextColor(d.HDC, win.COLORREF(fg))
			r.Left += 6
			r.Right -= 6
			win.DrawTextEx(d.HDC, &t[0], -1, &r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS, nil)
			win.SelectObject(d.HDC, of)
			if d.ItemState&win.ODS_FOCUS != 0 {
				win.DrawFocusRect(d.HDC, &r)
			}
			return 1
		}
	}
	r, _, _ := defSubclass.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func buttonIdealSize(hwnd uintptr, msg uint32, wp, lp, id, dpi uintptr) uintptr {
	if msg == win.WM_MOUSEMOVE {
		if dpi&buttonHot == 0 {
			setSubclass.Call(hwnd, buttonSizeProc, id, dpi|buttonHot)
			tracking := win.TRACKMOUSEEVENT{DwFlags: win.TME_LEAVE, HwndTrack: win.HWND(hwnd)}
			tracking.CbSize = uint32(unsafe.Sizeof(tracking))
			win.TrackMouseEvent(&tracking)
			win.InvalidateRect(win.HWND(hwnd), nil, false)
		}
		if wp&win.MK_LBUTTON == 0 {
			return 0
		}
	} else if msg == win.WM_MOUSELEAVE {
		setSubclass.Call(hwnd, buttonSizeProc, id, dpi&^buttonHot)
		win.InvalidateRect(win.HWND(hwnd), nil, false)
	}
	dpi &^= buttonHot
	if msg == win.BCM_GETIDEALSIZE && lp != 0 {
		hdc := win.GetDC(win.HWND(hwnd))
		defer win.ReleaseDC(win.HWND(hwnd), hdc)
		font := win.SendMessage(win.HWND(hwnd), win.WM_GETFONT, 0, 0)
		old := win.SelectObject(hdc, win.HGDIOBJ(font))
		defer win.SelectObject(hdc, old)
		var text [512]uint16
		getText.Call(hwnd, uintptr(unsafe.Pointer(&text[0])), 512)
		var r win.RECT
		win.DrawTextEx(hdc, &text[0], -1, &r, win.DT_CALCRECT|win.DT_SINGLELINE, nil)
		size := (*win.SIZE)(unsafe.Pointer(lp))
		size.CX = r.Right + int32(24*dpi/96)
		size.CY = r.Bottom + int32(12*dpi/96)
		return 1
	}
	r, _, _ := defSubclass.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func styleButtons(primary *walk.PushButton, buttons ...*walk.PushButton) {
	var primaryHandle uintptr
	if primary != nil {
		primaryHandle = uintptr(primary.Handle())
	}
	parents := map[win.HWND]bool{}
	for _, b := range buttons {
		_ = b.GraphicsEffects().Clear()
		b.SetBackground(walk.NullBrush())
		empty, _ := syscall.UTF16PtrFromString("")
		syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme").Call(uintptr(b.Handle()), uintptr(unsafe.Pointer(empty)), uintptr(unsafe.Pointer(empty)))
		h := b.Handle()
		p := win.GetParent(h)
		if !parents[p] {
			setSubclass.Call(uintptr(p), buttonProc, 1, primaryHandle)
			parents[p] = true
		}
		setSubclass.Call(uintptr(h), buttonSizeProc, 3, uintptr(b.DPI()))
		s := win.GetWindowLong(h, win.GWL_STYLE)
		win.SetWindowLong(h, win.GWL_STYLE, (s&^0xf)|win.BS_OWNERDRAW)
		win.InvalidateRect(h, nil, true)
	}
}

func (a *nativeUI) meter(w **walk.CustomWidget, value func() int) CustomWidget {
	return CustomWidget{AssignTo: w, MinSize: Size{0, 8}, MaxSize: Size{0, 8}, PaintMode: PaintNoErase, InvalidatesOnResize: true, Paint: func(c *walk.Canvas, _ walk.Rectangle) error {
		if *w == nil {
			return nil
		}
		size := (*w).ClientBounds()
		b, e := walk.NewSolidColorBrush(canvasColor)
		if e != nil {
			return e
		}
		defer b.Dispose()
		_ = c.FillRoundedRectanglePixels(b, walk.Rectangle{Width: size.Width, Height: 8}, walk.Size{Width: 8, Height: 8})
		n := value()
		if n <= 0 {
			return nil
		}
		color := quotaColor(n)
		f, e := walk.NewSolidColorBrush(color)
		if e != nil {
			return e
		}
		defer f.Dispose()
		fillW := size.Width * n / 100
		if fillW < 8 && n > 0 {
			fillW = 8
		}
		return c.FillRoundedRectanglePixels(f, walk.Rectangle{Width: fillW, Height: 8}, walk.Size{Width: 8, Height: 8})
	}}
}

func styleDarkControls(form walk.Form, widgets ...walk.Widget) {
	var dark uint32 = 1
	syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(uintptr(form.Handle()), 20, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	theme, _ := syscall.UTF16PtrFromString("DarkMode_Explorer")
	setTheme := syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme")
	for _, widget := range widgets {
		if _, ok := widget.(*walk.ComboBox); ok {
			theme, _ = syscall.UTF16PtrFromString("DarkMode_CFD")
		} else {
			theme, _ = syscall.UTF16PtrFromString("DarkMode_Explorer")
		}
		setTheme.Call(uintptr(widget.Handle()), uintptr(unsafe.Pointer(theme)), 0)
	}
}

func quotaColor(n int) walk.Color {
	if n <= 5 {
		return dangerColor
	}
	if n <= 20 {
		return warningColor
	}
	return accentColor
}

package cview

import (
	"fmt"
	"sync"

	"github.com/gdamore/tcell"
)

type WindowEdge int16

// Available mouse actions.
const (
	WindowEdgeNone WindowEdge = iota
	WindowEdgeTop
	WindowEdgeRight
	WindowEdgeBottom
	WindowEdgeLeft
	WindowEdgeBottomRight
	WindowEdgeBottomLeft
)

type WindowButtonSide int16

const (
	WindowButtonLeft = iota
	WindowButtonRight
)

const WindowZTop = -1
const WindowZBottom = 0

const minWindowWidth = 3
const minWindowHeight = 3

type WindowButton struct {
	Symbol       rune
	offsetX      int
	offsetY      int
	Alignment    int
	ClickHandler func()
}

// flexItem holds layout options for one item.
type Window struct {
	*Box
	root          Primitive // The item to be positioned. May be nil for an empty item.
	manager       *WindowManager
	buttons       []*WindowButton
	restoreX      int
	restoreY      int
	restoreWidth  int
	restoreHeight int
	maximized     bool
	Draggable     bool
	Resizable     bool
}

// NewWindow creates a new window in this window manager
func NewWindow() *Window {
	window := &Window{
		Box: NewBox().SetBackgroundColor(tcell.ColorDefault),
	}
	window.restoreX, window.restoreY, window.restoreHeight, window.restoreWidth = window.GetRect()
	window.SetBorder(true)
	window.focus = window
	return window
}

func (w *Window) SetRoot(root Primitive) *Window {
	w.root = root
	return w
}

func (w *Window) GetRoot() Primitive {
	return w.root
}

func (w *Window) Draw(screen tcell.Screen) {
	if w.Box.HasFocus() && !w.HasFocus() {
		w.Box.Blur()
	}
	w.Box.Draw(screen)

	if w.root != nil {
		x, y, width, height := w.GetInnerRect()
		w.root.SetRect(x, y, width, height)
		w.root.Draw(NewClipRegion(screen, x, y, width, height))
	}

	if w.border {
		x, y, width, height := w.GetRect()
		screen = NewClipRegion(screen, x, y, width, height)
		for _, button := range w.buttons {
			buttonX, buttonY := button.offsetX+x, button.offsetY+y
			if button.offsetX < 0 {
				buttonX += width
			}
			if button.offsetY < 0 {
				buttonY += height
			}

			//screen.SetContent(buttonX, buttonY, button.Symbol, nil, tcell.StyleDefault.Foreground(tcell.ColorYellow))
			Print(screen, Escape(fmt.Sprintf("[%c]", button.Symbol)), buttonX-1, buttonY, 9, 0, tcell.ColorYellow)
		}
	}
}

func (w *Window) checkManager() {
	if w.manager == nil {
		panic("Window must be added to a Window Manager to call this method")
	}
}

func (w *Window) Show() *Window {
	w.checkManager()
	w.manager.Show(w)
	return w
}

func (w *Window) Hide() *Window {
	w.checkManager()
	w.manager.Hide(w)
	return w
}

func (w *Window) Maximize() *Window {
	w.checkManager()
	w.restoreX, w.restoreY, w.restoreHeight, w.restoreWidth = w.GetRect()
	w.SetRect(w.manager.GetInnerRect())
	w.maximized = true
	return w
}

func (w *Window) Restore() *Window {
	w.SetRect(w.restoreX, w.restoreY, w.restoreHeight, w.restoreWidth)
	w.maximized = false
	return w
}

func (w *Window) ShowModal() *Window {
	w.checkManager()
	w.manager.ShowModal(w)
	return w
}

func (w *Window) Center() *Window {
	w.checkManager()
	mx, my, mw, mh := w.manager.GetInnerRect()
	x, y, width, height := w.GetRect()
	x = mx + (mw-width)/2
	y = my + (mh-height)/2
	w.SetRect(x, y, width, height)
	return w
}

// Focus is called when this primitive receives focus.
func (w *Window) Focus(delegate func(p Primitive)) {
	if w.root != nil {
		delegate(w.root)
		w.Box.Focus(nil)
	} else {
		delegate(w.Box)
	}
}

func (w *Window) Blur() {
	if w.root != nil {
		w.root.Blur()
	}
	w.Box.Blur()
}

func (w *Window) IsMaximized() bool {
	return w.maximized
}

// HasFocus returns whether or not this primitive has focus.
func (w *Window) HasFocus() bool {
	if w.root != nil {
		return w.root.GetFocusable().HasFocus()
	} else {
		return w.Box.HasFocus()
	}
}

func (w *Window) MouseHandler() func(action MouseAction, event *tcell.EventMouse, setFocus func(p Primitive)) (consumed bool, capture Primitive) {
	return w.WrapMouseHandler(func(action MouseAction, event *tcell.EventMouse, setFocus func(p Primitive)) (consumed bool, capture Primitive) {
		if action == MouseLeftClick {
			x, y := event.Position()
			wx, wy, width, _ := w.GetRect()
			if y == wy {
				for _, button := range w.buttons {
					if button.offsetX >= 0 && x == wx+button.offsetX || button.offsetX < 0 && x == wx+width+button.offsetX {
						if button.ClickHandler != nil {
							button.ClickHandler()
						}
						return true, nil
					}
				}
			}
		}
		if w.root != nil {
			return w.root.MouseHandler()(action, event, setFocus)
		}
		return false, nil
	})
}

func (w *Window) AddButton(button *WindowButton) *Window {
	w.buttons = append(w.buttons, button)

	offsetLeft, offsetRight := 2, -3
	for _, button := range w.buttons {
		if button.Alignment == AlignRight {
			button.offsetX = offsetRight
			offsetRight -= 3
		} else {
			button.offsetX = offsetLeft
			offsetLeft += 3
		}
	}

	return w
}

func (w *Window) GetButton(i int) *WindowButton {
	if i < 0 || i >= len(w.buttons) {
		return nil
	}
	return w.buttons[i]
}

func (w *Window) ButtonCount() int {
	return len(w.buttons)
}

type WindowManager struct {
	*Box

	// The windows to be positioned.
	windows []*Window

	mouseWindow              *Window
	dragOffsetX, dragOffsetY int
	draggedWindow            *Window
	draggedEdge              WindowEdge
	modalWindow              *Window
	sync.Mutex
}

// NewFlex returns a new flexbox layout container with no primitives and its
// direction set to FlexColumn. To add primitives to this layout, see AddItem().
// To change the direction, see SetDirection().
//
// Note that Box, the superclass of Flex, will have its background color set to
// transparent so that any nil flex items will leave their background unchanged.
// To clear a Flex's background before any items are drawn, set it to the
// desired color:
//
//   flex.SetBackgroundColor(cview.Styles.PrimitiveBackgroundColor)
func NewWindowManager() *WindowManager {
	wm := &WindowManager{
		Box: NewBox().SetBackgroundColor(tcell.ColorDefault),
	}
	wm.focus = wm
	return wm
}

// NewWindow creates a new window in this window manager
func (wm *WindowManager) NewWindow() *Window {
	window := NewWindow()
	window.manager = wm
	return window
}

func (wm *WindowManager) Show(window *Window) *WindowManager {
	wm.Lock()
	defer wm.Unlock()
	for _, wnd := range wm.windows {
		if wnd == window {
			return wm
		}
	}
	window.manager = wm
	wm.windows = append(wm.windows, window)
	return wm
}

func (wm *WindowManager) ShowModal(window *Window) *WindowManager {
	wm.Show(window)
	wm.Lock()
	defer wm.Unlock()
	wm.modalWindow = window
	return wm
}

func (wm *WindowManager) Hide(window *Window) *WindowManager {
	wm.Lock()
	defer wm.Unlock()
	if window == wm.modalWindow {
		wm.modalWindow = nil
	}
	for i, wnd := range wm.windows {
		if wnd == window {
			wm.windows = append(wm.windows[:i], wm.windows[i+1:]...)
			break
		}
	}
	return wm
}

func (wm *WindowManager) FindPrimitive(p Primitive) *Window {
	wm.Lock()
	defer wm.Unlock()
	for _, window := range wm.windows {
		if window.root == p {
			return window
		}
	}
	return nil
}

func (wm *WindowManager) WindowCount() int {
	wm.Lock()
	defer wm.Unlock()
	return len(wm.windows)
}

func (wm *WindowManager) Window(i int) *Window {
	wm.Lock()
	defer wm.Unlock()
	if i < 0 || i >= len(wm.windows) {
		return nil
	}
	return wm.windows[i]
}

func (wm *WindowManager) getZ(window *Window) int {
	for i, wnd := range wm.windows {
		if wnd == window {
			return i
		}
	}
	return -1
}
func (wm *WindowManager) GetZ(window *Window) int {
	wm.Lock()
	defer wm.Unlock()
	return wm.getZ(window)
}

func (wm *WindowManager) setZ(window *Window, newZ int) {
	oldZ := wm.getZ(window)
	lenW := len(wm.windows)
	if oldZ == -1 {
		return
	}

	if newZ < 0 || newZ >= lenW {
		newZ = lenW - 1
	}

	newWindows := make([]*Window, lenW)
	for i, j := 0, 0; i < lenW; j++ {
		if j == oldZ {
			j++
		}
		if i == newZ {
			j--
		} else {
			newWindows[i] = wm.windows[j]
		}
		i++
	}

	newWindows[newZ] = window
	wm.windows = newWindows
}

func (wm *WindowManager) SetZ(window *Window, newZ int) *WindowManager {
	wm.Lock()
	defer wm.Unlock()
	wm.setZ(window, newZ)
	return wm
}

// Draw draws this primitive onto the screen.
func (wm *WindowManager) Draw(screen tcell.Screen) {
	wm.Box.Draw(screen)

	wm.Lock()
	defer wm.Unlock()

	lenW := len(wm.windows)
	if lenW > 1 {
		for i, window := range wm.windows {
			if window.HasFocus() && i != lenW-1 {
				wm.setZ(window, WindowZTop)
				break
			}
		}
	}

	for _, window := range wm.windows {
		mx, my, mw, mh := wm.GetInnerRect()
		x, y, w, h := window.GetRect()
		if x < mx {
			x = mx
		}
		if y < my {
			y = my
		}

		if w < minWindowWidth {
			w = minWindowWidth
		}
		if h < minWindowHeight {
			h = minWindowHeight
		}

		if w > mw || window.maximized {
			w = mw
			x = mx
		}
		if h > mh || window.maximized {
			h = mh
			y = my
		}

		if x+w > mx+mw {
			x = mx + mw - w
		}

		if y+h > my+mh {
			y = my + mh - h
		}

		window.SetRect(x, y, w, h)
		window.Draw(screen)
	}
}

// Focus is called when this primitive receives focus.
func (wm *WindowManager) Focus(delegate func(p Primitive)) {
	wm.Lock()
	if len(wm.windows) > 0 {
		window := wm.windows[0]
		wm.Unlock()
		delegate(window)
		return
	}
	wm.Unlock()
}

func (wm *WindowManager) SetRect(x, y, width, height int) {
	wm.Box.SetRect(x, y, width, height)
	wm.Lock()
	defer wm.Unlock()
	for _, window := range wm.windows {
		_, _, windowWidth, windowHeight := window.GetRect()
		window.SetRect(x+window.x, y+window.y, windowWidth, windowHeight)
	}
}

// HasFocus returns whether or not this primitive has focus.
func (wm *WindowManager) HasFocus() bool {
	wm.Lock()
	defer wm.Unlock()

	for i := len(wm.windows) - 1; i >= 0; i-- {
		window := wm.windows[i]
		if window.GetFocusable().HasFocus() {
			return true
		}
	}
	return false
}

// MouseHandler returns the mouse handler for this primitive.
func (wm *WindowManager) MouseHandler() func(action MouseAction, event *tcell.EventMouse, setFocus func(p Primitive)) (consumed bool, capture Primitive) {
	return wm.WrapMouseHandler(func(action MouseAction, event *tcell.EventMouse, setFocus func(p Primitive)) (consumed bool, capture Primitive) {
		if !wm.InRect(event.Position()) {
			return false, nil
		}
		wm.Lock()

		if wm.draggedWindow != nil {
			switch action {
			case MouseLeftUp:
				wm.draggedWindow = nil
			case MouseMove:
				x, y := event.Position()
				wx, wy, ww, wh := wm.draggedWindow.GetRect()
				if wm.draggedEdge == WindowEdgeTop && wm.draggedWindow.Draggable {
					wm.draggedWindow.SetRect(x-wm.dragOffsetX, y-wm.dragOffsetY, ww, wh)
				} else {
					if wm.draggedWindow.Resizable {
						switch wm.draggedEdge {
						case WindowEdgeRight:
							wm.draggedWindow.SetRect(wx, wy, x-wx+1, wh)
						case WindowEdgeBottom:
							wm.draggedWindow.SetRect(wx, wy, ww, y-wy+1)
						case WindowEdgeLeft:
							wm.draggedWindow.SetRect(x, wy, ww+wx-x, wh)
						case WindowEdgeBottomRight:
							wm.draggedWindow.SetRect(wx, wy, x-wx+1, y-wy+1)
						case WindowEdgeBottomLeft:
							wm.draggedWindow.SetRect(x, wy, ww+wx-x, y-wy+1)
						}
					}
				}
				wm.Unlock()
				return true, nil
			}
		}

		var windows []*Window
		if wm.modalWindow != nil {
			windows = []*Window{wm.modalWindow}
		} else {
			windows = wm.windows
		}

		// Pass mouse events along to the first child item that takes it.
		for i := len(windows) - 1; i >= 0; i-- {
			window := windows[i]
			if !window.InRect(event.Position()) {
				continue
			}

			if action == MouseLeftDown && window.border {
				if !window.HasFocus() {
					setFocus(window)
				}
				wx, wy, ww, wh := window.GetRect()
				x, y := event.Position()
				wm.draggedEdge = WindowEdgeNone
				switch {
				case y == wy+wh-1:
					switch {
					case x == wx:
						wm.draggedEdge = WindowEdgeBottomLeft
					case x == wx+ww-1:
						wm.draggedEdge = WindowEdgeBottomRight
					default:
						wm.draggedEdge = WindowEdgeBottom
					}
				case x == wx:
					wm.draggedEdge = WindowEdgeLeft
				case x == wx+ww-1:
					wm.draggedEdge = WindowEdgeRight
				case y == wy:
					wm.draggedEdge = WindowEdgeTop
				}
				if wm.draggedEdge != WindowEdgeNone {
					wm.draggedWindow = window
					wm.dragOffsetX = x - wx
					wm.dragOffsetY = y - wy
					wm.Unlock()
					return true, nil
				}
			}
			wm.Unlock()
			return window.MouseHandler()(action, event, setFocus)
		}
		wm.Unlock()

		return
	})
}

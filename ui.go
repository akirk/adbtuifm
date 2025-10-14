package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/darkhz/tview"
	"github.com/gdamore/tcell/v2"
	adb "github.com/zach-klippenstein/goadb"
	"golang.org/x/sync/semaphore"
)

type dirPane struct {
	row         int
	path        string
	apath       string
	dpath       string
	finput      string
	filter      bool
	hidden      bool
	focused     bool
	mode        ifaceMode
	table       *tview.Table
	plock       *semaphore.Weighted
	entry       *adb.DirEntry
	pathList    []*adb.DirEntry
	title       *tview.TextView
	sortMethod  sortData
	history     []string
	historyPos  int
	historyMode []ifaceMode
}

var (
	app      *tview.Application
	pages    *tview.Pages
	opsView  *tview.Table
	selPane  *dirPane
	auxPane  *dirPane
	prevPane *dirPane

	panes          *tview.Flex
	titleBar       *tview.Flex
	mainFlex       *tview.Flex
	wrapVertical   *tview.Flex

	boxVertical       *tview.Box
	boxTitleSeparator *tview.Box
	boxLogSeparator   *tview.Box

	appSuspend bool
)

func newDirPane(selpane bool) *dirPane {
	var initPath string
	var initMode ifaceMode

	if selpane {
		initMode = initSelMode
		initPath = initSelPath
	} else {
		initMode = initAuxMode
		initPath = initAuxPath
	}

	return &dirPane{
		mode:   initMode,
		path:   initPath,
		apath:  initAPath,
		dpath:  initLPath,
		table:  tview.NewTable(),
		title:  tview.NewTextView(),
		plock:  semaphore.NewWeighted(1),
		hidden: true,
	}
}

func newTextView() *tview.TextView {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetTextColor(tcell.ColorDefault)
	tv.SetBackgroundColor(tcell.ColorDefault)
	return tv
}

func setupUI() {
	app = tview.NewApplication()
	pages = tview.NewPages()

	pages.AddPage("main", setupPaneView(), true, true)
	pages.AddPage("ops", setupOpsView(), true, true)

	pages.SwitchToPage("main")

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			return nil

		case tcell.KeyCtrlD:
			execCmd("", "Foreground", "Local")

		case tcell.KeyCtrlZ:
			appSuspend = true
		}

		if event.Modifiers() != tcell.ModAlt {
			return event
		}

		switch event.Rune() {
		case 'd':
			execCmd("", "Foreground", "Adb")
		}

		return event
	})

	app.SetBeforeDrawFunc(func(t tcell.Screen) bool {
		width, _ := t.Size()

		suspendUI(t)
		resizePopup(width)
		resizeDirEntries(width)

		return false
	})

	if err := app.SetRoot(pages, true).SetFocus(prevPane.table).Run(); err != nil {
		panic(err)
	}
}

func setupPaneView() *tview.Flex {
	selPane, auxPane = newDirPane(true), newDirPane(false)

	prevPane = selPane

	setupStatus()

	selPane.focused = true
	auxPane.focused = false

	setupPane(selPane, auxPane, true)
	setupPane(auxPane, selPane, true)

	boxVertical = tview.NewBox().
		SetBackgroundColor(tcell.ColorDefault).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			centerX := x + width/2
			for cy := y; cy < y+height; cy++ {
				screen.SetContent(
					centerX,
					cy,
					tview.BoxDrawingsLightVertical,
					nil,
					tcell.StyleDefault.Foreground(tcell.ColorDefault),
				)
			}

			return x + 1, centerX + 1, width - 2, height - (centerX + 1 - y)
		})

	boxTitleSeparator = tview.NewBox().
		SetBackgroundColor(tcell.ColorDefault)

	boxLogSeparator = tview.NewBox().
		SetBackgroundColor(tcell.ColorDefault).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for cx := x; cx < x+width; cx++ {
				screen.SetContent(
					cx,
					y,
					tview.BoxDrawingsLightHorizontal,
					nil,
					tcell.StyleDefault.Foreground(tcell.ColorDefault),
				)
			}

			return x, y, width, height
		})

	panes = tview.NewFlex().
		AddItem(selPane.table, 0, 1, true).
		AddItem(boxVertical, 5, 0, false).
		AddItem(auxPane.table, 0, 1, false).
		SetDirection(tview.FlexColumn)

	titleBar = tview.NewFlex().
		AddItem(selPane.title, 0, 1, true).
		AddItem(boxTitleSeparator, 1, 0, true).
		AddItem(auxPane.title, 0, 1, false)

	wrapPanes := tview.NewFlex().
		AddItem(panes, 0, 2, true).
		SetDirection(tview.FlexRow)

	wrapView := tview.NewFlex().
		AddItem(wrapPanes, 0, 2, false)

	wrapFlex := tview.NewFlex().
		AddItem(wrapView, 0, 1, true)

	wrapVertical = tview.NewFlex().
		AddItem(titleBar, 1, 0, false).
		AddItem(wrapFlex, 0, 2, true).
		SetDirection(tview.FlexRow)

	logViewFlex := setupLogView()

	mainFlex = tview.NewFlex().
		AddItem(wrapVertical, 0, 1, true).
		AddItem(boxLogSeparator, 1, 0, false).
		AddItem(logViewFlex, 10, 0, false).
		AddItem(statuspgs, 1, 0, false).
		SetDirection(tview.FlexRow)

	wrapFlex.SetBackgroundColor(tcell.ColorDefault)

	return mainFlex
}

func setupOpsView() *tview.Flex {
	opsView = tview.NewTable()
	opsTitle := newTextView()

	opsFlex := tview.NewFlex().
		AddItem(opsTitle, 1, 0, false).
		AddItem(opsView, 0, 1, true).
		SetDirection(tview.FlexRow)

	exit := func() {
		if opsView.HasFocus() {
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			opsView.SetSelectable(false, false)
		}
	}

	canceltask := func() {
		row, _ := opsView.GetSelection()
		ref := opsView.GetCell(row, 0).GetReference()

		if ref != nil {
			ref.(*operation).cancelOps()
		}
	}

	opsView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			exit()
		}

		switch event.Rune() {
		case 'x':
			canceltask()

		case 'X':
			cancelAllOps()

		case 'o':
			exit()

		case 'q':
			pages.SwitchToPage("main")
			stopApp()
		}

		return event
	})

	opsView.SetSelectable(true, false)

	opsTitle.SetText("[::bu]Operations")

	opsView.SetBorderColor(tcell.ColorDefault)
	opsView.SetBackgroundColor(tcell.ColorDefault)

	return opsFlex
}

//gocyclo:ignore
func setupPane(selPane, auxPane *dirPane, loadDir bool) {
	selPane.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		prevPane = selPane

		switch event.Key() {
		case tcell.KeyEscape:
			reset(selPane, auxPane)

		case tcell.KeyTab, tcell.KeyBacktab:
			paneswitch(selPane, auxPane)

		case tcell.KeyCtrlO:
			go selPane.openFileHandler()

		case tcell.KeyCtrlR:
			selPane.reselect(true)

		case tcell.KeyEnter, tcell.KeyRight:
			selPane.ChangeDirEvent(true, false)
			return nil

		case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyLeft:
			selPane.ChangeDirEvent(false, true)
			return nil
		}

		switch event.Rune() {
		case 'o':
			opsPage()

		case 'q':
			stopApp()

		case '?':
			showHelp()

		case 'h', '.':
			selPane.setHidden()

		case '/':
			selPane.showFilterInput()

		case ';':
			selPane.showSortDirInput()

		case 's', '<':
			selPane.modeSwitchHandler()

		case 'g', '>':
			selPane.showChangeDirInput()
			return nil

		case 'r':
			selPane.ChangeDir(false, false)

		case 'S':
			showEditSelections(nil)

		case 'l':
			showFullscreenLog()

		case '[':
			go selPane.navigateHistory(false)

		case ']':
			go selPane.navigateHistory(true)

		case '!':
			execCommand()

		case 'A', 'a', ' ':
			multiselect(selPane, event.Rune())

		case 'm', 'p', 'P', 'd':
			opsHandler(selPane, auxPane, event.Rune())

		case 'M', 'R':
			showMkdirRenameInput(selPane, auxPane, event.Rune())
		}

		return event
	})

	selPane.table.SetBorder(false)
	selPane.table.SetSelectorWrap(true)
	selPane.table.SetSelectable(true, false)
	selPane.table.SetBackgroundColor(tcell.ColorDefault)

	selPane.title.SetDynamicColors(true)
	selPane.title.SetTextAlign(tview.AlignCenter)
	selPane.title.SetTextColor(tcell.ColorDefault)
	selPane.title.SetBackgroundColor(tcell.ColorDefault)

	selPane.table.SetSelectionChangedFunc(func(row, col int) {
		rows := selPane.table.GetRowCount()

		if rows == 0 || row < 0 || row > rows {
			return
		}

		for c := 0; c < 3; c++ {
			cell := selPane.table.GetCell(row, c)
			if cell == nil {
				continue
			}

			cell.SetSelectedStyle(tcell.Style{}.
				Attributes(tcell.AttrReverse))
		}

		cell := selPane.table.GetCell(row, 0)
		if cell != nil {
			ref := cell.GetReference()
			if ref != nil {
				dir := ref.(*adb.DirEntry)
				if dir.Name != ".." {
					msgchan <- message{tview.Escape(dir.Name), true}
				}
			}
		}
	})

	if loadDir {
		selPane.ChangeDir(false, false)
	}
}

func suspendUI(t tcell.Screen) {
	if !appSuspend {
		return
	}

	t.Suspend()
	syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
	t.Resume()

	appSuspend = false
}

func opsPage() {
	rows := opsView.GetRowCount()

	if rows == 0 {
		showInfoMsg("No operations in queue")
		return
	}

	app.SetFocus(opsView)
	pages.SwitchToPage("ops")
	opsView.SetSelectable(true, false)
}

func paneswitch(selPane, auxPane *dirPane) {
	selPane.focused = false
	auxPane.focused = true

	auxPane.reselect(false)
	app.SetFocus(auxPane.table)
	selPane.table.SetSelectable(false, false)
	auxPane.table.SetSelectable(true, false)
}

func reset(selPane, auxPane *dirPane) {
	selected = false
	multiselection = make(map[string]ifaceMode)

	selPane.focused = true
	auxPane.focused = false

	selPane.table.SetSelectable(false, false)
	selPane.reselect(true)

	if selPane.mode == auxPane.mode &&
		selPane.getPath() == auxPane.getPath() {
		auxPane.reselect(true)
	}

	app.SetFocus(selPane.table)
	selPane.table.SetSelectable(true, false)
	auxPane.table.SetSelectable(false, false)
}

func resetOpsView() {
	count := opsView.GetRowCount()
	row, _ := opsView.GetSelection()

	switch {
	case count/opRowNum == 0:
		if opsView.HasFocus() {
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			opsView.SetSelectable(false, false)
		}

	case row-1 == count:
		opsView.Select(row-opRowNum, 0)
	}
}

func multiselect(selPane *dirPane, key rune) {
	var all, inverse bool

	switch key {
	case 'A':
		all = true
		inverse = false

	case 'a':
		all = false
		inverse = true

	case ' ':
		all = false
		inverse = false
	}

	totalrows := selPane.table.GetRowCount()

	if totalrows <= 0 {
		return
	}

	selPane.multiSelectHandler(all, inverse, totalrows)
}

func (p *dirPane) reselect(force bool) {
	if !p.getLock() {
		return
	}
	defer p.setUnlock()

	if p.filter && !force {
		for row := 0; row < p.table.GetRowCount(); row++ {
			cell := p.table.GetCell(row, 0)
			if cell == nil {
				continue
			}

			ref := cell.GetReference()
			if ref == nil {
				continue
			}

			dir := ref.(*adb.DirEntry)

			checksel := checkSelected(p.path, dir.Name, false)
			p.updateDirPane(row, checksel, dir)
		}
	} else {
		var row int

		if p.path != "/" && p.path != "" {
			parentDir := &adb.DirEntry{
				Name: "..",
				Mode: os.ModeDir | 0755,
			}
			p.updateDirPane(row, false, parentDir)
			row++
		}

		for _, dir := range p.pathList {
			checksel := checkSelected(p.path, dir.Name, false)
			p.updateDirPane(row, checksel, dir)
			row++
		}

		p.filter = false
	}

	pos, _ := p.table.GetSelection()
	p.table.Select(pos, 0)
}

func (p *dirPane) updateDirPane(row int, sel bool, dir *adb.DirEntry) {
	entry := getListEntry(dir)

	perms := strings.ToLower(dir.Mode.String())
	if len(perms) > 10 {
		perms = perms[1:]
	}

	// For ".." parent directory, only show the name column
	isParentDir := dir.Name == ".."

	for col, dname := range entry {
		// Skip size and date columns for parent directory
		if isParentDir && col > 0 {
			continue
		}

		if col == 0 {
			mode := dir.Mode&os.ModeDir != 0
			if len(dname) > 0 && mode {
				dname += "/"
			}
		}

		color, attr := setEntryColor(col, sel, perms)

		cell := tview.NewTableCell(tview.Escape(dname))
		cell.SetReference(dir)
		cell.SetBackgroundColor(tcell.ColorDefault)

		if col > 0 {
			// Column 1: size (right-aligned)
			// Column 2: date (right-aligned)
			if col == 1 {
				cell.SetExpansion(1)
			}
			cell.SetAlign(tview.AlignRight)
		} else {
			cell.SetSelectable(true)
			_, _, w, _ := pages.GetRect()
			maxWidth := (w / 2) - 30
			if maxWidth < 30 {
				maxWidth = 30
			}
			cell.SetMaxWidth(maxWidth)
		}

		p.table.SetCell(row, col, cell.SetTextColor(color).
			SetAttributes(attr))
	}
}

func (p *dirPane) updateRef(lock bool) {
	update := func() {
		p.row, _ = p.table.GetSelection()

		ref := p.table.GetCell(p.row, 0).GetReference()

		if ref != nil {
			p.entry = ref.(*adb.DirEntry)
		} else {
			p.entry = nil
		}
	}

	if !lock {
		update()
		return
	}

	app.QueueUpdateDraw(func() {
		update()
	})
}

func (p *dirPane) setPaneTitle() {
	prefix := ""

	switch p.mode {
	case mAdb:
		prefix = "Adb"

	case mLocal:
		prefix = "Local"
	}

	switch {
	case p.path == "./" || p.path == "../":
		p.path = "/"

	default:
		p.path = trimPath(p.path, false)
	}

	dpath := tview.Escape(p.path)
	_, _, titleWidth, _ := p.title.GetRect()

	if len(dpath) > titleWidth {
		dir := trimPath(dpath, true)
		base := filepath.Base(dpath)

		dir = trimName(dir, titleWidth-len(base)-20, true)
		dpath = dir + base
	}

	p.title.SetText("[::bu]" + prefix + ": " + dpath)
}

func (p *dirPane) setPaneSelectable(status bool) {
	if status {
		p.table.SetSelectable(p.focused, false)
		return
	}

	app.QueueUpdateDraw(func() {
		p.table.SetSelectable(false, false)
	})
}

func (p *dirPane) setHidden() {
	if !p.getLock() {
		return
	}
	defer p.setUnlock()

	if p.hidden {
		p.hidden = false
		showInfoMsg("Showing hidden files")
	} else {
		p.hidden = true
		showInfoMsg("Hiding hidden files")
	}

	p.ChangeDir(false, false)
}

func (p *dirPane) getHidden() bool {
	return p.hidden
}

func (p *dirPane) setUnlock() {
	p.plock.Release(1)
}

func (p *dirPane) getLock() bool {
	return p.plock.TryAcquire(1)
}

func stopApp() {
	quitmsg := "Quit"

	istask := opsView.GetRowCount()
	if istask > 0 {
		quitmsg += " (jobs are still running)"
	}

	quitmsg += " (Y/n)?"

	showConfirmMsg(quitmsg, "y", func() {
		stopUI()
	}, func() {})
}

func stopUI() {
	app.Stop()
	stopStatus()
	cancelAllOps()
}

func showHelp() {
	var row int

	helpview := tview.NewTable()
	helpview.SetBackgroundColor(tcell.ColorDefault)

	helpview.SetSelectedStyle(tcell.Style{}.
		Attributes(tcell.AttrReverse))

	helpview.SetBorderColor(tcell.ColorDefault)

	mainText := map[string]string{
		"Switch between panes ":                 "Tab ",
		"Navigate between entries ":             "Up, Down",
		"CD highlighted entry ":                 "Enter, Right",
		"Change one directory back ":            "Backspace, Left",
		"Switch to operations page ":            "o",
		"View fullscreen log ":                  "l",
		"Switch between ADB/Local ":             "s, <",
		"Change to any directory ":              "g, >",
		"Toggle hidden files ":                  "h, .",
		"Execute command":                       "!",
		"Refresh ":                              "r",
		"Move ":                                 "m",
		"Paste/Put ":                            "p",
		"Delete ":                               "d",
		"Open files ":                           "Ctrl+o",
		"Make directory ":                       "M",
		"Rename files/folders ":                 "R",
		"Filter entries":                        "/",
		"Toggle filtering modes (normal/regex)": "/",
		"Sort entries":                          ";",
		"Clear filtered entries ":               "Ctrl+r",
		"Select one item ":                      "Space",
		"Invert selection ":                     "a",
		"Select all items ":                     "A",
		"Edit selection list ":                  "S",
		"Navigate back in history ":             "[",
		"Navigate forward in history ":          "]",
		"Reset selections ":                     "Esc",
		"Temporarily exit to shell ":            "Ctrl+d",
		"Quit ":                                 "q",
	}

	opnsText := map[string]string{
		"Navigate between entries ":  "Up, Down",
		"Cancel selected operation ": "x",
		"Cancel all operations ":     "X",
		"Switch to main page ":       "o, Esc",
	}

	cdirText := map[string]string{
		"Navigate between entries ": "Up, Down",
		"Autocomplete ":             "Tab",
		"CD to highlighted entry ":  "Enter",
		"Move back a directory ":    "Ctrl+w",
		"Switch to main page ":      "Esc",
	}

	editText := map[string]string{
		"Select one item ":     "Alt+Space",
		"Invert selection ":    "Alt+a",
		"Select all items ":    "Alt+A",
		"Save edited list ":    "Ctrl+s",
		"Cancel editing list ": "Esc",
	}

	execText := map[string]string{
		"Switch b/w Local/Adb ":       "Ctrl+a",
		"Switch b/w FG/BG execution ": "Ctrl+q",
	}

	helpview.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape, tcell.KeyEnter:
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			prevPane.table.SetSelectable(true, false)
		}

		switch event.Rune() {
		case 'q':
			pages.SwitchToPage("main")
			stopApp()
		}

		return event
	})

	helpview.SetSelectionChangedFunc(func(row, col int) {
		if row <= 4 {
			helpview.ScrollToBeginning()
		} else if row >= helpview.GetRowCount()-4 {
			helpview.ScrollToEnd()
		}
	})

	for i, helpMap := range []map[string]string{
		mainText,
		opnsText,
		cdirText,
		editText,
		execText,
	} {
		var header string

		switch i {
		case 0:
			header = "MAIN PAGE"

		case 1:
			header = "OPERATIONS PAGE"

		case 2:
			header = "CHANGE DIRECTORY MODE"

		case 3:
			header = "EDIT SELECTION MODE"

		case 4:
			header = "EXECUTION MODE"
		}

		helpview.SetCell(row, 0, tview.NewTableCell("[::b]["+header+"[]").
			SetExpansion(1).
			SetSelectable(false).
			SetTextColor(tcell.ColorDefault).
			SetAlign(tview.AlignCenter))

		helpview.SetCell(row, 1, tview.NewTableCell("").
			SetExpansion(0).
			SetTextColor(tcell.ColorDefault).
			SetSelectable(false))
		row++

		helpview.SetCell(row, 0, tview.NewTableCell("[::bu]Operation").
			SetExpansion(1).
			SetSelectable(false).
			SetTextColor(tcell.ColorDefault).
			SetAlign(tview.AlignLeft))

		helpview.SetCell(row, 1, tview.NewTableCell("[::bu]Key").
			SetExpansion(0).
			SetTextColor(tcell.ColorDefault).
			SetSelectable(false))
		row++

		for k, v := range helpMap {
			helpview.SetCell(row, 0, tview.NewTableCell(k).
				SetTextColor(tcell.ColorDefault))
			helpview.SetCell(row, 1, tview.NewTableCell(v).
				SetTextColor(tcell.ColorDefault))

			row++
		}
	}

	exitText := "----- Press Enter/Escape to exit -----"

	helpview.SetCell(row, 0, tview.NewTableCell(exitText).
		SetExpansion(1).
		SetSelectable(false).
		SetTextColor(tcell.ColorDefault).
		SetAlign(tview.AlignCenter))

	helpview.SetCell(row, 1, tview.NewTableCell("").
		SetExpansion(0).
		SetTextColor(tcell.ColorDefault).
		SetSelectable(false))

	helpview.SetEvaluateAllRows(true)

	app.SetFocus(helpview)
	helpview.SetSelectable(true, false)
	pages.AddAndSwitchToPage("help", helpview, true)
}

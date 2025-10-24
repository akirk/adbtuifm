package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/darkhz/tview"
	adb "github.com/zach-klippenstein/goadb"
)

type message struct {
	text    string
	persist bool
}

var (
	statuspgs *tview.Pages
	statusmsg *tview.TextView

	mrinput    string
	msgchan    chan message
	entrycache []string

	sctx    context.Context
	scancel context.CancelFunc
)

func startStatus() {
	var text string
	var cleared bool

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-sctx.Done():
			return

		case msg, ok := <-msgchan:
			if !ok {
				return
			}

			t.Reset(2 * time.Second)

			cleared = false

			if msg.persist {
				text = msg.text
			}

			app.QueueUpdateDraw(func() {
				if !msg.persist || (msg.text == "" && msg.persist) {
					statusmsg.SetText(msg.text)
				}
			})

		case <-t.C:
			if cleared {
				continue
			}

			cleared = true

			app.QueueUpdateDraw(func() {
				statusmsg.SetText(text)
			})
		}
	}
}

func stopStatus() {
	scancel()
	close(msgchan)
}

func sendMessage(msg message) {
	defer func() {
		if r := recover(); r != nil {
			// Channel is closed, ignore
		}
	}()

	select {
	case msgchan <- msg:
	case <-sctx.Done():
	}
}

func setupStatus() {
	statuspgs = tview.NewPages()

	statusmsg = newTextView()

	statuspgs.AddPage("statusmsg", statusmsg, true, true)

	statuspgs.SetBackgroundColor(tcell.ColorDefault)

	sctx, scancel = context.WithCancel(context.Background())

	msgchan = make(chan message)
	go startStatus()
}

func getStatusInput(msg string, accept bool) *tview.InputField {
	input := tview.NewInputField()

	input.SetLabel("[::b]" + msg + " ")

	if accept {
		input.SetAcceptanceFunc(tview.InputFieldMaxLength(1))
	}

	input.SetLabelColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)
	input.SetBackgroundColor(tcell.ColorDefault)
	input.SetFieldBackgroundColor(tcell.ColorDefault)

	return input
}

func showInfoMsg(msg string) {
	sendMessage(message{"[::b]" + tview.Escape(msg), false})
}

func showErrorMsg(err error, autocomplete bool) {
	if autocomplete {
		return
	}

	sendMessage(message{"[red::b]" + tview.Escape(err.Error()), false})
}

func showConfirmMsg(msg string, defaultChoice string, doFunc, resetFunc func()) {
	input := getStatusInput(msg, true)

	exit := func(reset bool) {
		if reset {
			resetFunc()
		}

		statuspgs.SwitchToPage("statusmsg")
		app.SetFocus(prevPane.table)
	}

	infomsg := func() {
		info := strings.Fields(msg)[0]

		info = opString(info)

		if info == "" {
			return
		}

		info += " items"

		sendMessage(message{info, false})
	}

	confirm := func() {
		var reset bool

		text := input.GetText()
		input.SetText("")

		// Use default if no input provided
		if text == "" {
			text = defaultChoice
		}

		switch text {
		case "y":
			doFunc()
			infomsg()

			reset = true
			fallthrough

		case "n":
			// Must be async so input handler can return before UI operations
			go func() {
				exit(reset)
			}()

		default:
			return
		}
	}

	input.SetChangedFunc(func(text string) {
		if text == "" {
			return
		}

		switch text {
		case "y", "n":
			return

		default:
			input.SetText("")
		}
	})

	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			confirm()
		case tcell.KeyEscape:
			exit(false)
		}
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape, tcell.KeyLeft, tcell.KeyRight:
			exit(false)
			return nil

		case tcell.KeyUp, tcell.KeyDown:
			exit(false)
			prevPane.table.InputHandler()(event, nil)
			return nil
		}

		return event
	})

	statuspgs.AddAndSwitchToPage("confirm", input, true)
	app.SetFocus(input)
}

func (p *dirPane) showFilterInput() {
	var regex bool
	var skipCallback bool

	input := getStatusInput("", false)

	inputlabel := func() {
		var mode string

		if regex {
			mode = "regex"
		} else {
			mode = "normal"
		}

		label := fmt.Sprintf("[::b]Filter (%s): ", mode)
		input.SetLabel(label)
	}

	exit := func() {
		p.finput = input.GetText()
		statuspgs.SwitchToPage("statusmsg")
		app.SetFocus(prevPane.table)
	}

	input.SetChangedFunc(func(text string) {
		if skipCallback {
			return
		}
		go func() {
			if !p.getLock() {
				return
			}
			defer p.setUnlock()

			type filteredEntry struct {
				row int
				dir *adb.DirEntry
				sel bool
			}
			var filtered []filteredEntry

			if text == "" {
				// No filter - show all files
				p.filter = false
				var row int
				if p.path != "/" && p.path != "" {
					parentDir := &adb.DirEntry{
						Name: "..",
						Mode: os.ModeDir | 0755,
					}
					filtered = append(filtered, filteredEntry{row, parentDir, false})
					row++
				}
				for _, dir := range p.pathList {
					sel := checkSelected(p.path, dir.Name, false)
					filtered = append(filtered, filteredEntry{row, dir, sel})
					row++
				}
			} else {
				// Filter mode
				p.filter = true
				for _, dir := range p.pathList {
					match := false
					if regex {
						re, err := regexp.Compile(text)
						if err != nil {
							return
						}
						match = re.Match([]byte(dir.Name))
					} else {
						match = strings.Contains(
							strings.ToLower(dir.Name),
							strings.ToLower(text),
						)
					}

					if match {
						sel := checkSelected(p.path, dir.Name, false)
						filtered = append(filtered, filteredEntry{len(filtered), dir, sel})
					}
				}
			}

			app.QueueUpdateDraw(func() {
				p.table.Clear()
				for _, entry := range filtered {
					p.updateDirPane(entry.row, entry.sel, entry.dir)
				}
				p.table.Select(0, 0)
				p.table.ScrollToBeginning()
			})
		}()
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlR:
			go p.reselect(true)

		case tcell.KeyCtrlF:
			regex = !regex
			inputlabel()

		case tcell.KeyUp, tcell.KeyDown:
			p.table.InputHandler()(event, nil)
			// Must be async so input handler can return before UI operations
			go exit()
			return nil

		case tcell.KeyEnter, tcell.KeyEscape:
			// Must be async so input handler can return before UI operations
			go exit()
			return nil
		}

		return event
	})

	inputlabel()

	statuspgs.AddAndSwitchToPage("filter", input, true)
	app.SetFocus(input)

	skipCallback = true
	input.SetText(p.finput)
	skipCallback = false
}

func showMkdirRenameInput(selPane, auxPane *dirPane, key rune) {
	var title string
	var rename bool

	row, _ := selPane.table.GetSelection()

	ref := selPane.table.GetCell(row, 0).GetReference()
	if ref == nil {
		return
	}

	origname := ref.(*adb.DirEntry).Name

	switch key {
	case 'M':
		rename = false
		title = "Make directory:"

	case 'R':
		rename = true
		title = "Rename To:"
	}

	exit := func() {
		statuspgs.SwitchToPage("statusmsg")
		app.SetFocus(selPane.table)
	}

	infomsg := func(newname string) {
		var info string

		newname, origname = tview.Escape(newname), tview.Escape(origname)

		switch key {
		case 'M':
			info = "Created '" + newname + "' in " + selPane.getPath()

		case 'R':
			info = "Renamed '" + origname + "' to '" + newname + "'"
		}

		sendMessage(message{info, false})
	}

	input := getStatusInput(title, false)
	if rename {
		input.SetText(origname)
	}

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp, tcell.KeyDown:
			prevPane.table.InputHandler()(event, nil)
			exit()

		case tcell.KeyEnter:
			text := input.GetText()
			if text != "" {
				mrinput = text
				opsHandler(selPane, auxPane, key)
			}

			infomsg(text)
			fallthrough

		case tcell.KeyEscape:
			exit()
		}

		return event
	})

	statuspgs.AddAndSwitchToPage("mrinput", input, true)
	app.SetFocus(input)
}

func (p *dirPane) showSortDirInput() {
	input := getStatusInput("", true)

	sortmethods := []string{
		"asc",
		"desc",
		"filetype",
		"date",
		"name",
	}

	inputlabel := func() {
		label := "[::b]Sort by: "
		sortmethod, arrange := p.getSortMethod()

		for _, st := range sortmethods {
			if st == sortmethod || st == arrange {
				label += "*"
			}

			if st == "date" {
				st = "da(t)e"
			} else {
				st = "(" + string(st[0]) + ")" + string(st[1:])
			}

			label += st + " "
		}

		input.SetLabel(label)
	}

	setsort := func(t rune) {
		var s, a string

		switch t {
		case 'a':
			a = sortmethods[0]

		case 'd':
			a = sortmethods[1]

		case 'f':
			s = sortmethods[2]

		case 't':
			s = sortmethods[3]

		case 'n':
			s = sortmethods[4]
		}

		p.setSortMethod(s, a)
		inputlabel()

		p.ChangeDir(false, false)
	}

	inputlabel()

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'a', 'd', 'f', 't', 'n':
			setsort(event.Rune())
			return nil
		}

		p.table.InputHandler()(event, nil)
		statuspgs.SwitchToPage("statusmsg")
		app.SetFocus(p.table)

		return event
	})

	statuspgs.AddAndSwitchToPage("sortinput", input, true)
	app.SetFocus(input)
}

func (p *dirPane) showChangeDirInput() {
	input := getStatusInput("Change Directory to:", false)
	input.SetText(p.path)

	changeDirSelect(p, input)

	statuspgs.AddAndSwitchToPage("cdinput", input, true)
	app.SetFocus(input)
}

func showEditSelections(sinput *tview.InputField) {
	input := getStatusInput("Filter selections:", false)

	input = editSelections(input, sinput)
	if input == nil {
		return
	}

	statuspgs.AddAndSwitchToPage("editsel", input, true)
	app.SetFocus(input)
}

func execCommand() {
	imode := "Local"
	emode := "Foreground"

	input := getStatusInput("", false)

	exit := func() {
		statuspgs.SwitchToPage("statusmsg")
		app.SetFocus(prevPane.table)
	}

	inputlabel := func() {
		label := fmt.Sprintf("[::b]Exec (%s, %s): ", imode, emode)
		input.SetLabel(label)
	}

	cmdexec := func(cmdtext string) {
		if cmdtext == "" {
			return
		}

		_, err := execCmd(cmdtext, emode, imode)
		if err != nil {
			showErrorMsg(err, false)
		}
	}

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlA:
			if imode == "Local" {
				imode = "Adb"
			} else {
				imode = "Local"
			}

			inputlabel()

		case tcell.KeyCtrlQ:
			if emode == "Foreground" {
				emode = "Background"
			} else {
				emode = "Foreground"
			}

			inputlabel()

		case tcell.KeyEnter:
			cmdexec(input.GetText())
			fallthrough

		case tcell.KeyEscape:
			exit()
		}

		return event
	})

	inputlabel()

	statuspgs.AddAndSwitchToPage("exec", input, true)
	app.SetFocus(input)
}

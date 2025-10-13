package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/darkhz/tview"
	"github.com/gdamore/tcell/v2"
)

type logEntry struct {
	timestamp time.Time
	command   string
	output    string
	isError   bool
}

var (
	logView    *tview.TextView
	logEntries []logEntry
	logMutex   sync.Mutex
)

func setupLogView() *tview.Flex {
	logView = tview.NewTextView()
	logTitle := tview.NewTextView()

	logView.SetDynamicColors(true)
	logView.SetScrollable(true)
	logView.SetWrap(true)
	logView.SetBackgroundColor(tcell.ColorDefault)

	logTitle.SetDynamicColors(true)
	logTitle.SetText("[::b]ADB Log")
	logTitle.SetBackgroundColor(tcell.ColorDefault)

	logFlex := tview.NewFlex().
		AddItem(logTitle, 1, 0, false).
		AddItem(logView, 0, 1, true).
		SetDirection(tview.FlexRow)

	return logFlex
}

func addLog(command string, output string, isError bool) {
	logMutex.Lock()
	defer logMutex.Unlock()

	entry := logEntry{
		timestamp: time.Now(),
		command:   command,
		output:    output,
		isError:   isError,
	}

	logEntries = append(logEntries, entry)

	updateLogView()
}

func clearLog() {
	logMutex.Lock()
	defer logMutex.Unlock()

	logEntries = nil
	updateLogView()
	showInfoMsg("Log cleared")
}

func updateLogView() {
	if logView == nil {
		return
	}

	app.QueueUpdateDraw(func() {
		logView.Clear()

		if len(logEntries) == 0 {
			fmt.Fprintf(logView, "[gray::i]No ADB commands yet. Press 'c' to clear this log.")
			return
		}

		for _, entry := range logEntries {
			timestamp := entry.timestamp.Format("15:04:05")
			color := "white"
			if entry.isError {
				color = "red"
			}

			fmt.Fprintf(logView, "[gray]%s[white] [%s::b]$ adb %s[-:-:-]\n",
				timestamp, color, tview.Escape(entry.command))

			if entry.output != "" {
				fmt.Fprintf(logView, "%s\n", tview.Escape(entry.output))
			}

			fmt.Fprintf(logView, "\n")
		}

		logView.ScrollToEnd()
	})
}

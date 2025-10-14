package main

import (
	"fmt"
	"os"
	"strings"
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
	logFile    *os.File
)

func setupLogView() *tview.Flex {
	var err error
	logFile, err = os.OpenFile("/tmp/adbtuifm-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}

	logView = newTextView()
	logTitle := newTextView()

	logView.SetScrollable(true)
	logView.SetWrap(true)

	logTitle.SetText("[::b]ADB Log")

	logFlex := tview.NewFlex().
		AddItem(logTitle, 1, 0, false).
		AddItem(logView, 0, 1, true).
		SetDirection(tview.FlexRow)

	return logFlex
}

func addLog(command string, output string, isError bool) {
	timestamp := time.Now().Format("15:04:05.000")

	// Write to file immediately (synchronously for debugging)
	if logFile != nil {
		fmt.Fprintf(logFile, "%s $ adb %s", timestamp, command)
		if output != "" {
			fmt.Fprintf(logFile, " - %s", output)
		}
		fmt.Fprintf(logFile, "\n")
		logFile.Sync()
	}

	go func() {
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
	}()
}

func startLog(command string) int {
	logMutex.Lock()
	defer logMutex.Unlock()

	entry := logEntry{
		timestamp: time.Now(),
		command:   command,
		output:    "[running...]",
		isError:   false,
	}

	logEntries = append(logEntries, entry)
	index := len(logEntries) - 1

	updateLogView()
	return index
}

func updateLog(index int, output string, isError bool) {
	logMutex.Lock()
	defer logMutex.Unlock()

	if index >= 0 && index < len(logEntries) {
		logEntries[index].output = output
		logEntries[index].isError = isError
		updateLogView()
	}
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
			fmt.Fprintf(logView, "No ADB commands yet. Press 'c' to clear this log.")
			return
		}

		for _, entry := range logEntries {
			timestamp := entry.timestamp.Format("15:04:05.000")
			color := ""
			if entry.isError {
				color = "red"
			}

			fmt.Fprintf(logView, "%s [%s::b]$ adb %s[-:-:-]",
				timestamp, color, tview.Escape(entry.command))

			if entry.output != "" {
				if len(entry.output) < 50 && !strings.Contains(entry.output, "\n") {
					fmt.Fprintf(logView, " - %s\n", tview.Escape(entry.output))
				} else {
					fmt.Fprintf(logView, "\n%s\n", tview.Escape(entry.output))
				}
			} else {
				fmt.Fprintf(logView, "\n")
			}
		}

		logView.ScrollToEnd()
	})
}

func showFullscreenLog() {
	logMutex.Lock()
	defer logMutex.Unlock()

	fullscreenLogView := newTextView()
	fullscreenLogView.SetScrollable(true)
	fullscreenLogView.SetWrap(true)

	if len(logEntries) == 0 {
		fmt.Fprintf(fullscreenLogView, "No ADB commands yet.")
	} else {
		for _, entry := range logEntries {
			timestamp := entry.timestamp.Format("15:04:05.000")
			color := ""
			if entry.isError {
				color = "red"
			}

			fmt.Fprintf(fullscreenLogView, "%s [%s::b]$ adb %s[-:-:-]",
				timestamp, color, tview.Escape(entry.command))

			if entry.output != "" {
				if len(entry.output) < 50 && !strings.Contains(entry.output, "\n") {
					fmt.Fprintf(fullscreenLogView, " - %s\n", tview.Escape(entry.output))
				} else {
					fmt.Fprintf(fullscreenLogView, "\n%s\n", tview.Escape(entry.output))
				}
			} else {
				fmt.Fprintf(fullscreenLogView, "\n")
			}
		}
	}

	fullscreenLogView.ScrollToEnd()

	fullscreenLogView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			return nil
		}

		switch event.Rune() {
		case 'l', 'q':
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			return nil

		case 'c':
			clearLog()
			pages.SwitchToPage("main")
			app.SetFocus(prevPane.table)
			return nil
		}

		return event
	})

	logFlex := tview.NewFlex().
		AddItem(fullscreenLogView, 0, 1, true).
		SetDirection(tview.FlexRow)

	pages.AddAndSwitchToPage("fullscreenlog", logFlex, true)
	app.SetFocus(fullscreenLogView)
}

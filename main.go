package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	initAPath   string
	initLPath   string
	initSelPath string
	initAuxPath string
	initSelMode ifaceMode
	initAuxMode ifaceMode
)

func main() {
	cmdAPath := kingpin.Arg("remote-path", "Remote (ADB) path to start in").
		Default("/sdcard").String()

	kingpin.Parse()

	cwd, _ := os.Getwd()
	cmdLPath := cwd

	_, err := os.Lstat(cmdLPath)
	if err != nil {
		fmt.Printf("adbtuifm: %s: Invalid local path\n", cmdLPath)
		return
	}

	initSelMode = mLocal
	initSelPath, _ = filepath.Abs(cmdLPath)

	device, err := getAdb()
	if device == nil {
		fmt.Printf("adbtuifm: No ADB device connected\n")
		return
	}

	// Make remote path relative to /sdcard if it doesn't start with /
	adbPath := *cmdAPath
	if len(adbPath) > 0 && adbPath[0] != '/' {
		adbPath = filepath.Join("/sdcard", adbPath)
	}

	_, err = adbStat(device, adbPath)
	if err != nil {
		fmt.Printf("adbtuifm: %s: Invalid remote path\n", adbPath)
		return
	}

	initAuxMode = mAdb
	initAuxPath = adbPath

	initAPath = adbPath
	initLPath, _ = filepath.Abs(cmdLPath)

	jobNum = 0
	selected = false
	openFiles = make(map[string]struct{})
	multiselection = make(map[string]ifaceMode)

	sig := make(chan os.Signal, 1)
	signal.Notify(
		sig,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGTERM,
	)

	go func(s chan os.Signal) {
		switch <-s {
		case os.Interrupt:
			return
		default:
			stopUI()
			return
		}
	}(sig)

	setupUI()
}

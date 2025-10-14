package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	adb "github.com/zach-klippenstein/goadb"
)

var lastDeviceState adb.DeviceState

func checkAdb() bool {
	_, err := getAdb()
	if err != nil {
		showErrorMsg(err, false)
		return false
	}

	return true
}

func getAdb() (*adb.Device, error) {
	client, err := adb.NewWithConfig(adb.ServerConfig{})
	if err != nil {
		return nil, fmt.Errorf("ADB client not found")
	}

	device := client.Device(adb.AnyDevice())

	state, err := device.State()
	if state != lastDeviceState {
		addLog("devices", fmt.Sprintf("%v", state), err != nil || state != adb.StateOnline)
		lastDeviceState = state
	}
	if err != nil || state != adb.StateOnline {
		return nil, fmt.Errorf("ADB device not found")
	}

	return device, nil
}

func runAdbShellCommand(device *adb.Device, cmd string) (string, error) {
	logIndex := startLog(fmt.Sprintf("shell %s", cmd))
	out, err := device.RunCommand(cmd)
	updateLog(logIndex, out, err != nil)
	return out, err
}

func runAdbShellCommandContext(ctx context.Context, cmd string) (string, error) {
	logIndex := startLog(fmt.Sprintf("shell %s", cmd))
	out, err := exec.CommandContext(ctx, "adb", "shell", cmd).Output()
	updateLog(logIndex, string(out), err != nil)
	return string(out), err
}

func adbStat(device *adb.Device, path string) (*adb.DirEntry, error) {
	// Note: stat calls are deliberately not logged to avoid clogging the log
	// as they occur very frequently during normal navigation
	stat, err := device.Stat(path)
	return stat, err
}

func adbListDirEntries(device *adb.Device, path string) (*adb.DirEntries, error) {
	logIndex := startLog(fmt.Sprintf("ls %s", path))
	entries, err := device.ListDirEntries(path)
	updateLog(logIndex, "", err != nil)
	return entries, err
}

func isAdbSymDir(testPath, name string) bool {
	device, err := getAdb()
	if err != nil {
		return false
	}

	cmd := fmt.Sprintf("ls -pd %s%s/", testPath, name)
	out, err := runAdbShellCommand(device, cmd)

	if err != nil {
		return false
	}

	if !strings.HasSuffix(strings.TrimSpace(out), "//") {
		return false
	}

	return true
}

func (o *operation) adbOps(src, dst string) error {
	var err error

	device, err := getAdb()
	if err != nil {
		return err
	}

	switch o.transfer {
	case adbToAdb:
		err = o.execAdbCmd(src, dst, device)

	case localToAdb:
		err = o.pushRecursive(src, dst, device)

	case adbToLocal:
		err = o.pullRecursive(src, dst, device)
	}

	return err
}

func (o *operation) execAdbCmd(src, dst string, device *adb.Device) error {
	var cmd string

	srcfmt := fmt.Sprintf(" '%s'", src)
	dstfmt := fmt.Sprintf(" '%s'", dst)

	param := srcfmt + dstfmt

	switch o.opmode {
	case opMkdir:
		cmd = "mkdir"
		param = srcfmt

	default:
		stat, err := adbStat(device, src)
		if err != nil {
			return err
		}

		switch o.opmode {
		case opRename:
			_, err := adbStat(device, dst)
			if err == nil {
				return fmt.Errorf("rename %s %s: file exists", src, dst)
			}

			fallthrough

		case opMove:
			cmd = "mv"

		case opCopy:
			if stat.Mode.IsDir() {
				cmd = "cp -r"
			} else {
				cmd = "cp"
			}

		case opDelete:
			if stat.Mode.IsDir() {
				cmd = "rm -rf"
			} else {
				cmd = "rm"
			}

			param = srcfmt
		}
	}

	cmd = cmd + param
	out, err := runAdbShellCommandContext(o.ctx, cmd)

	if err != nil {
		if err.Error() == "signal: killed" {
			return error(context.Canceled)
		}

		return err
	}

	if out != "" {
		return fmt.Errorf(out)
	}

	return nil
}

func (p *dirPane) adbListDir(testPath string, autocomplete bool) ([]string, bool) {
	var dlist []string

	device, err := getAdb()
	if err != nil {
		showErrorMsg(err, autocomplete)
		return nil, false
	}

	_, err = adbStat(device, testPath)
	if err != nil {
		showErrorMsg(err, autocomplete)
		return nil, false
	}

	dent, err := adbListDirEntries(device, testPath)
	if err != nil {
		showErrorMsg(err, autocomplete)
		return nil, false
	}

	if !autocomplete {
		p.pathList = nil
	}

	for dent.Next() {
		ent := dent.Entry()
		name := ent.Name

		if name == ".." || name == "." {
			continue
		}

		if p.getHidden() && strings.HasPrefix(name, ".") {
			continue
		}

		if ent.Mode&os.ModeDir != 0 {
			dlist = append(dlist, filepath.Join(testPath, name))
		}

		if autocomplete {
			continue
		}

		p.pathList = append(p.pathList, ent)
	}
	if dent.Err() != nil {
		return nil, false
	}

	return dlist, true
}

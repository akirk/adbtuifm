package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dolmen-go/contextio"
	"github.com/schollz/progressbar/v3"
	adb "github.com/zach-klippenstein/goadb"
)

func (o *operation) sortEntries(list []*adb.DirEntry) {
	sort.Slice(list, func(i, j int) bool {
		var a, b int

		if list[i].Mode.IsDir() != list[j].Mode.IsDir() {
			return list[i].Mode.IsDir()
		}

		switch o.arrangeBy {
		case "asc":
			a, b = i, j

		case "desc":
			a, b = j, i
		}

		switch o.sortBy {
		case "filetype":
			if list[a].Mode.IsDir() || list[b].Mode.IsDir() {
				break
			}

			return filepath.Ext(list[a].Name) < filepath.Ext(list[b].Name)

		case "date":
			return list[a].ModifiedAt.Unix() < list[b].ModifiedAt.Unix()
		}

		return list[a].Name < list[b].Name
	})
}

func (o *operation) pullFile(src, dst string, entry *adb.DirEntry, device *adb.Device, recursive bool) error {
	remote, err := device.OpenRead(src)
	if err != nil {
		return err
	}
	defer remote.Close()

	local, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer local.Close()

	cioIn := contextio.NewReader(o.ctx, remote)
	prgIn := progressbar.NewReader(cioIn, o.progress.pbar)

	var logIndex int
	if !recursive {
		logIndex = startLog(fmt.Sprintf("pull %s %s", src, dst))
	}

	_, err = io.Copy(local, &prgIn)
	if err != nil {
		if !recursive {
			updateLog(logIndex, err.Error(), true)
		}
		return err
	}

	if !recursive {
		updateLog(logIndex, "success", false)
	}

	o.updatePb()

	return nil
}

func (o *operation) pullRecursive(src, dst string, device *adb.Device) error {
	select {
	case <-o.ctx.Done():
		return o.ctx.Err()

	default:
	}

	if o.opmode != opCopy {
		return fmt.Errorf("%s not implemented via pull", o.opmode.String())
	}

	stat, err := adbStat(device, src)
	if err != nil {
		return err
	}

	isDir := stat.Mode.IsDir()
	var logIndex int
	if isDir {
		logIndex = startLog(fmt.Sprintf("pull -r %s %s", src, dst))
	}

	if !isDir {
		return o.pullFile(src, dst, stat, device, false)
	}

	if err = os.MkdirAll(dst, stat.Mode); err != nil {
		return err
	}

	listIter, err := adbListDirEntries(device, src)
	var entries []*adb.DirEntry

	for listIter.Next() {
		entry := listIter.Entry()
		entries = append(entries, entry)
	}
	if listIter.Err() != nil {
		if isDir && logIndex >= 0 {
			updateLog(logIndex, listIter.Err().Error(), true)
		}
		return listIter.Err()
	}

	o.sortEntries(entries)

	for _, entry := range entries {
		s := filepath.Join(src, entry.Name)
		d := filepath.Join(dst, entry.Name)

		if entry.Mode&os.ModeDir != 0 {
			if err = o.pullRecursive(s, d, device); err != nil {
				return err
			}
			continue
		}

		if err = o.pullFile(s, d, entry, device, true); err != nil {
			return err
		}
	}

	if isDir && logIndex >= 0 {
		updateLog(logIndex, "success", false)
	}

	return nil
}

func (o *operation) pushFile(src, dst string, entry os.FileInfo, device *adb.Device, recursive bool) error {
	var err error

	addLog("pushFile", fmt.Sprintf("src=%s dst=%s size=%d", src, dst, entry.Size()), false)

	switch {
	case entry.Mode()&os.ModeSymlink != 0:
		src, err = filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}

	case entry.Mode()&os.ModeNamedPipe != 0:
		return nil
	}

	logIndex := startLog(fmt.Sprintf("push %s %s (%.1f MB)", src, dst, float64(entry.Size())/(1024*1024)))

	// Use script command to capture adb push with PTY for live progress
	// script -q /dev/null runs command with a PTY but discards the typescript file
	cmd := exec.CommandContext(o.ctx, "script", "-q", "/dev/null", "adb", "push", src, dst)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		addLog("pushFile", fmt.Sprintf("adb push start error: %v", err), true)
		return err
	}

	// Read output and capture progress (update UI less frequently)
	var lastProgress string
	go func() {
		buf := make([]byte, 512)
		for {
			n, readErr := stdoutPipe.Read(buf)
			if n > 0 {
				// Parse the output - look for percentage
				output := string(buf[:n])
				// Split by \r to get latest progress line
				parts := strings.Split(output, "\r")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" && strings.Contains(part, "%") {
						lastProgress = part
					}
				}
			}
			if readErr != nil {
				break
			}
		}
	}()

	// Update progress in UI periodically
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if lastProgress != "" {
					updateLog(logIndex, lastProgress, false)
				}
			case <-done:
				return
			}
		}
	}()

	err = cmd.Wait()
	close(done) // Stop the progress updater

	if err != nil {
		errMsg := fmt.Sprintf("error: %v", err)
		addLog("pushFile", fmt.Sprintf("adb push error: %v", err), true)
		updateLog(logIndex, errMsg, true)
		return err
	}

	addLog("pushFile", "adb push done", false)
	updateLog(logIndex, "success", false)

	o.updatePb()

	return nil
}

//gocyclo:ignore
func (o *operation) pushRecursive(src, dst string, device *adb.Device) error {
	addLog("pushRecursive", fmt.Sprintf("src=%s dst=%s", src, dst), false)

	select {
	case <-o.ctx.Done():
		return o.ctx.Err()

	default:
	}

	if o.opmode != opCopy {
		return fmt.Errorf("%s not implemented via push", o.opmode.String())
	}

	stat, err := os.Lstat(src)
	if err != nil {
		addLog("pushRecursive", fmt.Sprintf("Lstat error: %v", err), true)
		return err
	}

	isDir := stat.Mode().IsDir()
	addLog("pushRecursive", fmt.Sprintf("isDir=%v size=%d", isDir, stat.Size()), false)

	var logIndex int
	if isDir {
		logIndex = startLog(fmt.Sprintf("push -r %s %s", src, dst))
	}

	if !isDir {
		addLog("pushRecursive", "calling pushFile for single file", false)
		return o.pushFile(src, dst, stat, device, false)
	}

	srcfd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcfd.Close()

	cmd := fmt.Sprintf("mkdir '%s'", dst)
	out, err := runAdbShellCommand(device, cmd)

	if err != nil {
		return err
	} else if out != "" {
		return fmt.Errorf(out)
	}

	mode := fmt.Sprintf("%04o", stat.Mode().Perm())
	cmd = fmt.Sprintf("chmod %s '%s'", mode, dst)
	out, err = runAdbShellCommand(device, cmd)

	if err != nil {
		return err
	} else if out != "" {
		return fmt.Errorf(out)
	}

	oslist, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	var entries []*adb.DirEntry
	for _, entry := range oslist {
		var d adb.DirEntry
		d.Name = entry.Name()
		d.Mode = entry.Mode()
		d.Size = int32(entry.Size())
		d.ModifiedAt = entry.ModTime()
		entries = append(entries, &d)
	}

	o.sortEntries(entries)

	for _, entry := range entries {
		s := filepath.Join(src, entry.Name)
		d := filepath.Join(dst, entry.Name)

		if entry.Mode.IsDir() {
			if err = o.pushRecursive(s, d, device); err != nil {
				return err
			}
			continue
		}

		osEntry, _ := os.Lstat(s)
		if err = o.pushFile(s, d, osEntry, device, true); err != nil {
			if isDir && logIndex >= 0 {
				updateLog(logIndex, err.Error(), true)
			}
			return err
		}
	}

	if isDir && logIndex >= 0 {
		updateLog(logIndex, "success", false)
	}

	return nil
}

func (o *operation) copyFile(src, dst string, entry os.FileInfo, recursive bool) error {
	var err error

	switch {
	case entry.Mode()&os.ModeSymlink != 0:
		src, err = filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}

	case entry.Mode()&os.ModeNamedPipe != 0:
		return syscall.Mkfifo(dst, uint32(entry.Mode()))
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	cioIn := contextio.NewReader(o.ctx, srcFile)
	prgIn := progressbar.NewReader(cioIn, o.progress.pbar)

	_, err = io.Copy(dstFile, &prgIn)
	if err != nil {
		return err
	}

	o.updatePb()

	return nil
}

func (o *operation) copyRecursive(src, dst string) error {
	select {
	case <-o.ctx.Done():
		return o.ctx.Err()

	default:
	}

	stat, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if !stat.Mode().IsDir() {
		return o.copyFile(src, dst, stat, false)
	}

	srcfd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcfd.Close()

	if err := os.MkdirAll(dst, stat.Mode()); err != nil {
		return err
	}

	oslist, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	var entries []*adb.DirEntry
	for _, entry := range oslist {
		var d adb.DirEntry
		d.Name = entry.Name()
		d.Mode = entry.Mode()
		d.Size = int32(entry.Size())
		d.ModifiedAt = entry.ModTime()
		entries = append(entries, &d)
	}

	o.sortEntries(entries)

	for _, entry := range entries {
		s := filepath.Join(src, entry.Name)
		d := filepath.Join(dst, entry.Name)

		if entry.Mode.IsDir() {
			if err = o.copyRecursive(s, d); err != nil {
				return err
			}
			continue
		}

		osEntry, _ := os.Lstat(s)
		if err = o.copyFile(s, d, osEntry, true); err != nil {
			return err
		}
	}

	return nil
}

func (o *operation) getTotalFiles(src string) error {
	if o.totalFile > 0 || o.opmode != opCopy {
		return nil
	}

	if o.transfer == adbToAdb {
		return nil
	}

	if o.transfer == adbToLocal {
		device, err := getAdb()
		if err != nil {
			return err
		}

		cmd := fmt.Sprintf("find '%s' -type f | wc -l", src)
		out, err := runAdbShellCommand(device, cmd)

		if err != nil {
			return err
		}

		o.totalFile, err = strconv.Atoi(strings.TrimSuffix(out, "\n"))
		if err != nil {
			return err
		}

		cmd = fmt.Sprintf("du -d0 -sh '%s'", src)
		out, err = runAdbShellCommand(device, cmd)

		if err != nil {
			return err
		}

		o.totalBytes, err = getByteSize(strings.Fields(out)[0])
		if err != nil {
			return err
		}

		return nil
	}

	err := filepath.Walk(src, func(p string, entry os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() {
			o.totalFile++
			o.totalBytes += entry.Size()
		}

		return nil
	})

	return err
}

func getByteSize(str string) (int64, error) {
	var exp int
	var err error
	var size int64

	const unit = 1024
	const suffixes = "KMGTPE"

	num := str[:len(str)-1]
	suffix := str[len(str)-1:]

	for i := 0; i < len(suffixes); i++ {
		if string(suffixes[i]) == suffix {
			exp = i
			break
		}
	}

	if strings.Contains(num, ".") {
		num = strings.Split(num, ".")[0]
	}

	size, err = strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0, err
	}

	return int64(size) * int64(math.Pow(unit, float64(exp+1))), nil
}

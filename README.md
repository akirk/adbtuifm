

[![Go Report Card](https://goreportcard.com/badge/github.com/akirk/adbtuifm)](https://goreportcard.com/report/github.com/akirk/adbtuifm)
# adbtuifm

This is a fork of [darkhz/adbtuifm](https://github.com/darkhz/adbtuifm) (which looks abandoned with the last commit being three years ago in 2025) that makes this work better for me in macOS. Feel free to also use it.

![demo](demo/demo.gif)

adbtuifm is a TUI-based file manager for the Android Debug Bridge, to make transfers
between the device and client easier.

It has been tested only on Mac. Windows/Linux might be supported by [darkhz/adbtuifm](https://github.com/darkhz/adbtuifm).

# Features
- Multiselection support, with a selections editor

- Transferring files/folders between the device and the local machine

- Open files of any file type from the device or local machine

- Copy, move, and delete operations on the device and the local machine<br />separately

- View file operations separately on a different screen, with ability to monitor<br />progress and  cancel operation

- ADB command log panel showing all ADB commands with timestamps and output

- Execute commands on the device or local machine, with toggleable<br />foreground/background execution modes

- Filter entries in each directory

- Display file size and modification date for all entries

- Directory navigation history with back/forward support

- Rename files/folders or create directories

- Switch between adbtuifm and shell easily

- Change to any directory via an inputbox, with autocompletion support

# Installation
```
go install github.com/akirk/adbtuifm@latest
```
# Usage
```
adbtuifm [<remote-path>]

Arguments:
  [<remote-path>]     Remote (ADB) path to start in (default: /sdcard)
```

The local directory is always set to your current working directory.

**Note:** If the remote path doesn't start with `/`, it will be treated as relative to `/sdcard/`.

Examples:
```bash
# Start with default ADB path (/sdcard) and current directory
adbtuifm

# Start with a specific ADB directory using absolute path
adbtuifm /sdcard/Downloads

# Start with a relative ADB path (resolved to /sdcard/Downloads)
adbtuifm Downloads

# Start in a specific local directory with custom ADB path
cd ~/Documents
adbtuifm Music   # Opens /sdcard/Music on device
```

# Keybindings

## Main Page
|Operation                                 |Key                                                     |
|------------------------------------------|--------------------------------------------------------|
|Switch between panes                      |<kbd>Tab</kbd>                                          |
|Navigate between entries                  |<kbd>Up</kbd>/<kbd>Down</kbd>                           |
|Change directory to highlighted entry     |<kbd>Enter</kbd>/<kbd>Right</kbd>                       |
|Change one directory back                 |<kbd>Backspace</kbd>/<kbd>Left</kbd>                    |
|Switch to operations page                 |<kbd>o</kbd>                                            |
|Switch between ADB/Local (in each pane)   |<kbd>s</kbd>/<kbd><</kbd>                               |
|Change to any directory                   |<kbd>g</kbd>/<kbd>></kbd>                               |
|Toggle hidden files                       |<kbd>h</kbd>/<kbd>.</kbd>                               |
|Execute command                           |<kbd>!</kbd>                                            |
|Refresh                                   |<kbd>r</kbd>                                            |
|Move                                      |<kbd>m</kbd>                                            |
|Put/Paste (duplicate existing entry)      |<kbd>p</kbd>                                            |
|Put/Paste (don't duplicate existing entry)|<kbd>P</kbd>                                            |
|Delete                                    |<kbd>d</kbd>                                            |
|Open files                                |<kbd>Ctrl</kbd>+<kbd>o</kbd>                            |
|View fullscreen log                       |<kbd>l</kbd>                                            |
|Filter entries                            |<kbd>/</kbd>                                            |
|Toggle filtering modes (normal/regex)     |<kbd>Ctrl</kbd>+<kbd>f</kbd>                            |
|Sort entries                              |<kbd>;</kbd>                                            |
|Clear filtered entries                    |<kbd>Ctrl</kbd>+<kbd>r</kbd>                            |
|Select one item                           |<kbd>Space</kbd>                                        |
|Inverse selection                         |<kbd>a</kbd>                                            |
|Select all items                          |<kbd>A</kbd>                                            |
|Edit selection list                       |<kbd>S</kbd>                                            |
|Make directory                            |<kbd>M</kbd>                                            |
|Navigate back in history                  |<kbd>[</kbd>                                            |
|Navigate forward in history               |<kbd>]</kbd>                                            |
|Rename files/folders                      |<kbd>R</kbd>                                            |
|Reset selections                          |<kbd>Esc</kbd>                                          |
|Suspend to shell                          |<kbd>Ctrl</kbd>+<kbd>z</kbd>                            |
|Launch local/ADB shell                    |<kbd>Ctrl</kbd>+<kbd>d</kbd>/<kbd>Alt</kbd>+<kbd>d</kbd>|
|Help                                      |<kbd>?</kbd>                                            |
|Quit                                      |<kbd>q</kbd>                                            |

## Operations Page
|Operation                |Key                          |
|-------------------------|-----------------------------|
|Navigate between entries |<kbd>Up</kbd>/<kbd>Down</kbd>|
|Cancel selected operation|<kbd>x</kbd>                 |
|Cancel all operations    |<kbd>X</kbd>                 |
|Switch to main page      |<kbd>o</kbd>/<kbd>Esc</kbd>  |

## Change Directory Selector
|Operation                            |Key                          |
|-------------------------------------|-----------------------------|
|Navigate between entries             |<kbd>Up</kbd>/<kbd>Down</kbd>|
|Autocomplete                         |<kbd>Tab</kbd>               |
|Change directory to highlighted entry|<kbd>Enter</kbd>             |
|Move back a directory                |<kbd>Ctrl</kbd>+<kbd>w</kbd> |
|Switch to main page                  |<kbd>Esc</kbd>               |

## Selections Editor
|Operation          |Key                            |
|-------------------|-------------------------------|
|Select one item    |<kbd>Alt</kbd>+<kbd>Space</kbd>|
|Inverse selection  |<kbd>Alt</kbd>+<kbd>a</kbd>    |
|Select all items   |<kbd>Alt</kbd>+<kbd>A</kbd>    |
|Save edited list   |<kbd>Ctrl</kbd>+<kbd>s</kbd>   |
|Cancel editing list|<kbd>Esc</kbd>                 |

## Execution mode
|Operation                                     |Key                         |
|----------------------------------------------|----------------------------|
|Switch between Local/Adb execution            |<kbd>Ctrl</kbd>+<kbd>a</kbd>|
|Switch between Foreground/Background execution|<kbd>Ctrl</kbd>+<kbd>q</kbd>|

# Notes
- As of v0.5.5, keybindings have been revised and the UI has been revamped.<br />

- **Only Copy operations are cancellable**. Move and Delete operations will persist.<br />

- The current method to open files is via **xdg-open**. In certain cases, after opening<br /> and modifying a file, the application may take time to exit, and as a result no operations<br /> can be performed on the currently edited file until the application exits. For example, after<br /> opening a zip file via file-roller, modifying it and closing the file-roller GUI, file-roller takes some<br /> time to fully exit, and since the UI is waiting for file-roller to exit, the user cannot perform operations<br /> on the currently modified file until file-roller exits.

# Bugs
-  In directories with a huge amount of entries, autocompletion will lag.
   This happens only on the device side (i.e ADB mode), where there is
   significant latency in transferring and processing the directory listing
   to the client.

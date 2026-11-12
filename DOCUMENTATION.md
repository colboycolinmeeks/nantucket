# Nantucket Documentation

Nantucket is a dBase III / Clipper compatible programming environment written
in Go. It runs `.prg` programs, works with DBF tables and NTX indexes, and has
the classic interactive **dot prompt** and a full-screen program editor. It
also supports many Harbour and xHarbour language extensions.

This document covers installing and running Nantucket, the language, every
command, the SET options, colours and themes, the screen and GET system,
databases, the function library, error handling and testing. For a
command-by-command reference with examples, see [COMMANDS.md](COMMANDS.md).

---

## Contents

1. [Installation](#1-installation)
2. [Running Nantucket](#2-running-nantucket)
3. [The dot prompt](#3-the-dot-prompt)
4. [The program editor](#4-the-program-editor)
5. [Programs and files](#5-programs-and-files)
6. [Language reference](#6-language-reference)
7. [The preprocessor](#7-the-preprocessor)
8. [Command reference](#8-command-reference)
9. [SET reference](#9-set-reference)
10. [Colours and themes](#10-colours-and-themes)
11. [Screen, keyboard and the GET system](#11-screen-keyboard-and-the-get-system)
12. [Databases](#12-databases)
13. [Function reference](#13-function-reference)
14. [Error handling](#14-error-handling)
15. [Testing programs](#15-testing-programs)
16. [Header files](#16-header-files)
17. [Environment variables](#17-environment-variables)
18. [Compatibility notes and limitations](#18-compatibility-notes-and-limitations)
19. [Development](#19-development)

---

## 1. Installation

Nantucket needs Go 1.25 or later.

```sh
git clone <repository> nantucket
cd nantucket
make build          # produces ./nantucket
make install        # optional: installs into GOBIN (or GOPATH/bin)
```

Without `make`:

```sh
go build -o nantucket ./cmd/nantucket
```

### Make targets

| Target | What it does |
|---|---|
| `make` / `make all` | Check formatting, run `go vet`, run tests, build, then run the `.prg` tests |
| `make build` | Build `./nantucket` |
| `make test` | Run all tests |
| `make test-v` | Run all tests verbosely |
| `make test-prg` | Build, then run the example `.prg` tests with `nantucket test examples` |
| `make test-race` | Run tests with the race detector |
| `make cover` | Run tests and write `coverage.html` |
| `make vet` | Run `go vet` |
| `make fmt` | Format the Go source with `gofmt` |
| `make fmt-check` | Fail if any Go source needs formatting |
| `make tidy` | Run `go mod tidy` |
| `make run` | Build and start the dot prompt |
| `make demo` | Build and run the guided tour, `examples/demo.prg` |
| `make example` | Build and run `examples/contacts.prg` |
| `make dist` | Build a release archive in `dist/` with the binary, `LICENSE`, `NOTICE`, docs and examples (set `GOOS`/`GOARCH` to cross-compile) |
| `make dist-all` | Release archives for Linux, macOS and Windows |
| `make install` | Install the binary with `go install` |
| `make clean` | Remove build outputs and the test cache |
| `make help` | List the targets |

---

## 2. Running Nantucket

```
nantucket [options] [program[.prg] [args...]]
```

| Usage | Effect |
|---|---|
| `nantucket` | Start the interactive dot prompt |
| `nantucket myprog` | Run `myprog.prg`; extra arguments are passed to it as parameters |
| `nantucket -c 'commands'` | Run the commands and exit. Separate commands with newlines or `;` |
| `nantucket -stream myprog` | Use plain line-by-line output instead of the full screen |
| `nantucket -q myprog` | Don't wait for a key when the program ends |
| `nantucket -theme mono` | Choose a colour theme (`color`, `classic`, `mono`) |
| `nantucket test [paths]` | Run the `*_test.prg` tests. See [Testing programs](#15-testing-programs) |
| `nantucket build myprog` | Package `myprog.prg` as a single executable. See [Packaging a program](#packaging-a-program) |

### Options

| Option | Description |
|---|---|
| `-c <text>` | Execute commands and exit |
| `-stream` | Plain line output. Useful for pipes, scripts and CI |
| `-q` | Don't show "Press any key to exit..." when a program finishes |
| `-theme <name>` | Colour theme. See [Colours and themes](#10-colours-and-themes) |
| `-trace <file>` | Write every procedure call and program line, with its source text, to a file (`-` for standard error). See [Tracing](#tracing-a-program) |
| `-debug` | Open the debugger at the program's first line. See [Debugging](#debugging-a-program) |
| `-cpuprofile <file>` | Write a Go CPU profile (for developers) |

### Full screen and stream consoles

Nantucket has two consoles:

- **Full screen** is used when both standard input and output are terminals.
  It supports `@ ... SAY/GET`, `READ`, `BROWSE`, `MENU TO`, the editor and
  colours.
- **Stream** is used with `-stream`, or when input or output is redirected.
  Output is written line by line and keys are read from standard input.
  Commands that need the full screen (`BROWSE`, `EDIT`, `APPEND`, `CREATE`,
  `MODIFY STRUCTURE`, `MODIFY COMMAND`, `DBEdit()`) raise the error
  "... requires an interactive terminal".

When a program fails in stream mode, the error report is printed to standard
error.

### Tracing a program

Three ways to follow what a program does, useful when moving an old
program across:

- `nantucket -trace trace.txt myprog` writes every procedure call and
  every program line, with its source text, to `trace.txt` (indented by
  call depth). `-trace -` writes to standard error.
- `SET ECHO ON` in a program shows each line on screen as it runs, as
  dBase did; `SET ECHO OFF` stops.
- `SET STEP ON` shows each line and waits for a key before running it;
  Esc turns stepping off.

```
calc.prg(1) CALC: x := Twice(21)
  -> TWICE(1 argument(s))
  calc.prg(5) TWICE: RETURN n * 2
calc.prg(2) CALC: ? x
```

Lines typed at the dot prompt are not traced.

### Debugging a program

The debugger stops a running program and shows where it is and what its
variables hold. Open it:

- by pressing **Alt-D** while a program runs (or waits for a key);
- from the program, by calling `AltD()`;
- at the first line, by starting the program with `nantucket -debug
  myprog`.

```
 Debugger   Step   DOUBLE  calc.prg(12)
     11  FUNCTION Double(n)
 ►   12     LOCAL nOut := n * 2
     13     RETURN nOut
─ Variables ──────────────────────────┬─ Call stack ─────────
 N            LOCAL   1               │ DOUBLE(12)  calc.prg
 NOUT         LOCAL   NIL             │ MAIN(5)  calc.prg
 CNAME        PRIVATE "Ann"           │
> Type an expression to see it, x := 5 to change x, WATCH <expr>, BP <line>
 F5 Go  F8 Step  F10 Over  F6 Out  F7 Cursor  F9 Break  F4 Screen  Tab  F1 Help
```

The top part is the source, with `►` at the line about to run and `●` at
breakpoints. Below are the variables (watches, then the `LOCAL` and
`STATIC` variables of the selected call, then the `PRIVATE` and `PUBLIC`
variables in sight) and the call stack. Tab moves between the three.

| Key | Action |
|---|---|
| F5 | Go: run until a breakpoint or the end |
| F8 | Step: run one line, going into procedures it calls |
| F10 | Step over: run one line, without stopping in the procedures it calls |
| F6 | Step out: run until the current procedure returns |
| F7 | Run to the line with the cursor |
| F9 | Set or clear a breakpoint on the line with the cursor; in the variables, set or clear a watchpoint on the selected variable |
| F3 | Show the log written by logpoints |
| F4 | Show the program's screen until a key is pressed |
| F1 | Help |

In the call stack, the arrows select another call: its source and its
`LOCAL` variables are shown. In the variables, Enter on an array or hash
lists its elements, and Enter on another variable puts `name := value` on
the command line to change it.

Typing anything opens the command line, which evaluates expressions in
the selected call, `LOCAL` variables included:

| Command | Action |
|---|---|
| `<expr>` or `? <expr>` | Show the value of an expression |
| `x := 5` | Change a variable |
| `WATCH <expr>` | Show an expression's value at every stop (Del removes it; `UNWATCH` removes them all) |
| `BP [line]` | Set or clear a breakpoint on a line of the file shown (default: the cursor's) |
| `BP <name>` | Stop when a procedure or function is called |
| `BP [line \| name] IF <condition>` | Stop there only when the condition is true |
| `BP [line \| name] HIT <n>` | Stop on the nth hit only; `HIT >= n` from the nth on, `HIT % n` every nth |
| `LP [line \| name] "<text>" [IF <c>] [HIT <n>]` | Logpoint: log the text, with each `{expr}` replaced by its value, instead of stopping (`LP <line>` clears it) |
| `LOG` / `LOG CLEAR` | Show (as F3 does) / empty the log |
| `LOGFILE [<file>]` | Also append the log to a file (`.log`); `LOGFILE` alone stops |
| `DB <alias->field> [IF <c>] [HIT <n>]` | Data breakpoint: stop when a field changes in any record (`alias->*` for any field; `DB` on the same field again clears it) |
| `WP <expr>` | Watchpoint: stop when the expression's value changes (`WP <expr>` again removes it) |
| `BL` / `BC` | List (with their hits so far) / clear all breakpoints, logpoints, watchpoints and data breakpoints |
| `GO`, `STEP` | As F5 and F8 |
| `QUIT` | Stop the program |

A conditional breakpoint stops only when its condition is true, for
example `BP 12 IF nTotal > 1000` or `BP CalcTax IF nAmount < 0`. The
condition is evaluated in the call being stopped, so it can use that
procedure's `LOCAL` variables and parameters. It is marked `◆` in the
source (an ordinary breakpoint is `●`), and its condition is shown when
the cursor is on its line. `BP ... IF` on a line that already has a
breakpoint replaces its condition; `BP <line>` or F9 clears it. A
condition that can't be evaluated, or doesn't give a logical value, stops
the program and says why, so a mistake in it isn't hidden.

A hit-count breakpoint stops only on some of the times it is reached:
`BP 12 HIT 50` stops the 50th time line 12 runs, `BP 12 HIT >= 50` the
50th time and every time after, and `BP CalcTax HIT % 10` every 10th
call. With a condition as well (`BP 12 IF nAmount < 0 HIT 3`, IF and HIT
in either order), only the times the condition is true are counted, so
that one stops at the third negative amount. These are marked `◆` too;
the title bar says which hit stopped the program, and with the cursor on
the line the debugger shows how many hits there have been. Counts start
again at zero each time a program is run from the dot prompt.

A logpoint writes a message to the debugger's log instead of stopping
the program, so you can follow what a program does without stepping
through it:

```
LP 12 "customer {cName}, balance {nBalance}"
LP CalcTax "called with {nAmount}" IF nAmount < 0
LP 30 "{Len(aItems)} items" HIT % 100
```

Each `{expression}` is replaced by its value in the call that reaches the
logpoint, so `LOCAL` variables and parameters can be used; text values
appear without quotes, and `{{` and `}}` stand for braces. Put the
message in quotes (`"` or `'`); without them it runs to the end of the
command, and `IF` and `HIT` can't follow it. `IF` and `HIT` work as for
breakpoints. A logpoint is marked `◇` in the source. The log holds the
last 1,000 lines, each starting with where it was logged
(`calc.prg(12) CALCTAX: called with -5`). F3 (or `LOG`) shows it, and
when the debugger next opens it says how many lines are new.
`LOGFILE trace` also appends the log to `trace.log`, and with
`nantucket -trace` the log goes into the trace too. The log is never
written to the program's screen. A message whose expression fails logs
the error in its place; a condition that fails logs the error instead of
the message.

A data breakpoint stops the program when a field of a table changes, in
any record and however it is written: `REPLACE`, an assignment such as
`customer->balance := 0`, `APPEND FROM`, a `GET`, `BROWSE`...

```
DB customer->balance
DB balance                          (a field of the current work area)
DB customer->* IF Trim(state) == "MA"
DB customer->balance HIT >= 100
```

The debugger says which field and record changed and what from and to
(`Data breakpoint: CUSTOMER->BALANCE, record 2, 50.00 to 100.00`), and
shows the line that made the change. A `REPLACE ALL` stops at each
record it changes; F5 goes on to the next. Writing a field's own value
back isn't a change. A condition is evaluated in the program's current
call with the changed record current, so it can test the record's other
fields. The table must be open when the data breakpoint is set; it then
applies to that table's file in whatever work area it is used. Changes
made in the debugger, or typed at the dot prompt, don't stop. Unlike
watchpoints, data breakpoints don't slow the program down: only field
writes are checked.

A watchpoint stops the program when a value changes: `WP nTotal`, or F9
on a variable in the variables pane. The debugger says what it changed
from and to, and which line changed it (for a change made by a procedure
that was called, the line that called it), for example `Watchpoint:
nTotal changed from 0 to 2 at calc.prg(5)`. The program stops before the
next line runs, so the source shows the line after the change. Any
expression can be watched (`WP Len(aItems)`, `WP customer->balance`), a
change to an element of an array counts, and a variable that doesn't
exist yet shows as `(undefined)` until it is created. Watchpoints are
marked `◉` at the top of the variables pane; Del removes one. A
watchpoint that uses `LOCAL` variables belongs to the call it was set in
and is removed when that call returns (the debugger says so). Changes
made in the debugger itself, such as `nTotal := 10`, don't stop the
program. Watched expressions are checked before every line, so a program
with watchpoints runs much more slowly.

Errors in an expression are shown on the command line; they don't reach
the program's error handler. Breakpoints stay set for the rest of the
session, so a program can be run again with `DO` and stop at the same
places. `AltD(0)` turns Alt-D off (so the program receives the key) and
`AltD(1)` turns it back on. Programs run at full speed until a breakpoint
is set or the debugger is opened.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success, or the program ended with `QUIT` |
| 1 | A runtime or compile error, or the program was cancelled |
| 2 | Startup problem: program file not found, unknown theme, or the runtime failed to start |
| 129, 143 | Ended by SIGHUP (the terminal was closed) or SIGTERM (`kill`, system shutdown) |

When Nantucket is told to end (SIGTERM, or SIGHUP when its terminal
window is closed), it stops the program at the next line, or while it is
waiting for a key, and saves every changed record and index before
exiting. If it can't reach a safe point within three seconds, or is killed
outright (`kill -9`, a power cut), indexes that missed changes are rebuilt
the next time they are opened.

### Packaging a program

`nantucket build` turns a program into a single executable that runs
without Nantucket installed:

```sh
nantucket build invoice            # writes ./invoice
nantucket build -o bin/inv invoice.prg
./invoice March                    # arguments go to the program
```

It packages `invoice.prg` and the files it uses, found by following
`DO <name>`, `SET PROCEDURE TO <file>` and `#include` from file to file
(looking next to each file, then next to the main program). Every program
is compiled first, so errors are reported at build time and nothing is
written. Headers built into Nantucket (`inkey.ch` and the rest) don't need
packaging.

| Option | Description |
|---|---|
| `-o <file>` | Output file (default: the program's name, plus `.exe` for a Windows runtime) |
| `-with <files>` | Extra files to include, comma separated or repeated. Use it for programs the scan can't see, such as `DO &cName` |
| `-runtime <file>` | The Nantucket executable to build on (default: the one running). Use a runtime from `make dist-all` to package for another system |

Things to know:

- The executable is a copy of Nantucket with the program files appended.
  It starts the program straight away; all its arguments go to the
  program. It uses the full screen on a terminal and plain lines
  otherwise, and honours `NANTUCKET_THEME` and `NANTUCKET_PALETTE`.
- Data files (`.dbf`, `.ntx`, `.dbt`, `.mem`) are not packaged. The program
  finds them in the directory it runs from, as usual.
- Packaged program files are used before any files of the same name on
  disk.
- **The source code is not hidden.** The program files are stored as an
  ordinary zip archive at the end of the executable, so anyone who has it
  can unzip it and read your `.prg` files. Don't put passwords or other
  secrets in a packaged program, and don't rely on packaging to protect
  code you sell.
- A packaged program contains Nantucket and the libraries it uses. If you
  give the executable to others, include Nantucket's `LICENSE`, `NOTICE`
  and `THIRD_PARTY_LICENSES` files with it.
- Packaging for macOS has not been tested. Appending data to a macOS
  executable can upset its code signature; if macOS refuses to run the
  result, re-sign it with `codesign --force -s - <file>`.
- The executable includes Nantucket, so it is covered by the Apache
  License: ship the `LICENSE` and `NOTICE` files with it.

### Interrupting a program

Press **Ctrl-C** or **Alt-C** to interrupt a running program. In stream mode
an interrupt signal (SIGINT) does the same thing. **Alt-D** opens the
[debugger](#debugging-a-program) instead.

### Example programs

```sh
./nantucket examples/demo
./nantucket examples/contacts
```

`examples/demo.prg` is a guided tour of the essentials: creating a table,
adding records, indexes and searching, queries and totals, `BROWSE`, data
entry forms, `EDIT`, menus and windows. A Guide window explains each step
and a Code window shows the xBase behind it. It works on a table called
`DEMOCUST.DBF` in the current directory and offers to delete it at the end.

`examples/contacts.prg` is a small address book that shows menus,
`@ ... SAY/GET`, `READ`, indexes and `BROWSE`.

---

## 3. The dot prompt

Running `nantucket` with no program starts the dot prompt:

```
Nantucket 0.2  -  dBase III / Clipper compatible
Type HELP for commands, VERSION FULL for what's new, QUIT to exit.
. USE customer
. LIST name, city FOR state = "MA"
. ? RecCount()
```

- Type any command or expression statement and press Enter.
- **Up/Down** recall earlier lines (history). History is kept between
  sessions in `~/.nantucket_history` (the last 1,000 lines). Set
  `NANTUCKET_HISTORY` to use another file, or to `off` to keep none.
- Control structures (`IF`, `DO WHILE`, `FOR`, `DO CASE`, ...) can be typed
  over several lines. The prompt waits until the structure is closed before
  running it.
- `!<command>` or `RUN <command>` runs a shell command. `!` on its own opens
  a shell.
- `CD <dir>` changes the current directory (`CD` alone shows it, `CD -`
  goes back). `!cd <dir>` does the same: it changes Nantucket's directory,
  not just a shell's.
- `HELP` opens a summary of the commands in a window that stays on screen
  while you work. Scroll it with the mouse wheel; drag it to move it; `[x]`
  closes it. See [Windows](#windows).
- Without a mouse: **F1** opens or closes `HELP`, **Esc** on an empty line
  closes the `HELP` or `VERSION FULL` window in front, and **F6** lets you
  arrange windows with the keyboard (see [Windows](#windows)).
- `VERSION` prints the version number. `VERSION FULL` opens a window with
  the version and what's new in it; like `HELP`, it stays on screen while
  you work.
- The bottom line is a status bar: work area, table, record number and total
  records, controlling index, filter, current directory, and the date (in
  the `SET DATE` format) and time. `SET STATUS OFF` hides it.
- `QUIT` leaves Nantucket.
- Errors are reported with a description and the call trace. The prompt
  stays open.

---

## 4. The program editor

`MODIFY COMMAND <name>` (short form `MODI COMM`) opens `<name>.prg` in the
full-screen editor. `MODIFY FILE <name>` edits any text file. The same editor
is used by `MemoEdit()` and for memo fields in `BROWSE` and `EDIT`.

The editor highlights xBase syntax in `.prg` and `.ch` files. When you save a
`.prg` and exit, Nantucket compiles it. If there is an error the file is
still saved, but the editor stays open with the cursor on the faulty line
and the message in the status line; fix it and save again, or press Ctrl-W
again to leave anyway.

Typing replaces a Shift selection, and Backspace or Del deletes it. A
Ctrl-K block stays marked while you move around.
`DO <name>` reloads a program automatically if its file has changed.

| Key | Action |
|---|---|
| Arrows, PgUp, PgDn | Move |
| Home | First non-blank character, then column 0 |
| End | End of line |
| Ctrl-Left / Ctrl-Right | Word left / right |
| Ctrl-PgUp (Ctrl-Home) / Ctrl-PgDn | Top / bottom of the file |
| Ins | Toggle insert and overtype |
| Enter | Split the line (keeps the indent) |
| Backspace / Del | Delete character |
| Tab / Shift-Tab | Indent / outdent |
| Ctrl-Y | Delete line |
| Ctrl-N | Insert line |
| Ctrl-T | Delete word |
| Ctrl-Z, Ctrl-U | Undo (typing is undone a run at a time) |
| Shift + arrows, Home, End, PgUp, PgDn | Select text |
| Ctrl-Ins / Shift-Del / Shift-Ins | Copy / cut / paste the selection |
| Ctrl-K B, Ctrl-K K | Mark the start / end of a block (WordStar and dBase keys) |
| Ctrl-K C / V / Y / H | Copy the block to the cursor / move it / delete it / hide it |
| F7 | Find |
| Ctrl-L | Find again |
| F5 | Go to line |
| F2 | Save and keep editing |
| Ctrl-W, Ctrl-End, F10 | Save and exit |
| Esc, Ctrl-Q | Exit (asks before discarding changes) |
| F1 | Help |

---

## 5. Programs and files

### Running programs

- `nantucket myprog` from the shell, or `DO myprog` at the dot prompt, runs
  `myprog.prg`.
- `DO <proc> WITH <args>` passes arguments **by reference**, as in dBase.
- A program's first function or procedure runs when the file is executed. If
  the file starts with statements outside any `FUNCTION`/`PROCEDURE`, those
  statements form the main procedure.
- `DO <name>` looks for a procedure or function called `<name>` that is
  already loaded, then for a file `<name>.prg`. If the loaded procedure came
  from `<name>.prg` and the file has changed since, it is recompiled first.
- `SET PROCEDURE TO <file> [ADDITIVE]` loads a file of procedures so they can
  be called from other programs.
- `COMPILE <file>` compiles a program without running it and reports errors.

### Where files are found

Programs, include files, tables and indexes are searched for in this order:

1. The `SET DEFAULT` directory, if set
2. The current directory
3. Each directory in `SET PATH` (separated by `;` or `,`)

File names are matched without regard to case, so `CUSTOMER.DBF` and
`customer.dbf` are both found. New files are created in the `SET DEFAULT`
directory, or the current directory if it isn't set.

### File types

| Extension | Contents |
|---|---|
| `.prg` | Program source |
| `.ch` | Preprocessor header |
| `.dbf` | Table |
| `.dbt` / `.fpt` | Memo file (dBase / FoxPro) |
| `.ntx` | Clipper index |
| `.mem` | Saved memory variables (`SAVE TO` / `RESTORE FROM`) |
| `.txt`, `.sdf` | Text data for `COPY TO` / `APPEND FROM` |
| `.prn` | Default printer output file |

---

## 6. Language reference

The language follows Clipper 5 and Harbour, with dBase III commands and many
xHarbour extensions.

### 6.1 Source text

- Commands and identifiers are **not case sensitive**.
- One statement per line. Put several statements on a line by separating
  them with `;`.
- Continue a long statement on the next line by ending the line with `;`.
- Commands and keywords can be shortened to their first **four letters**, as
  in dBase: `DISP STRU`, `MODI COMM`, `REPL`, `ENDD`, `RETU`.

### 6.2 Comments

```
* A whole-line comment (at the start of a line)
NOTE also a whole-line comment
x := 1   && comment to the end of the line
x := 2   // comment to the end of the line
/* block comment,
   can span lines */
```

### 6.3 Data types

| Type | `ValType()` | Examples |
|---|---|---|
| Character | `C` | `"text"`, `'text'`, `[text]` |
| Memo | `M` | The value of a memo field; behaves like Character |
| Numeric | `N` | `42`, `3.14`, `0x1F` (hex) |
| Date | `D` | `0d20261109`, `CToD("11/09/26")`, `SToD("20261109")` |
| Logical | `L` | `.T.`, `.F.`, `.Y.`, `.N.` |
| Array | `A` | `{1, "two", .T.}` |
| Hash | `H` | `{"name" => "Ann", "age" => 30}` |
| Code block | `B` | `{|x| x * 2}` |
| NIL | `U` | `NIL` |
| Object | `O` | Error objects |

A memo value keeps type `M` when it is copied into a variable. Anything
built from it, such as `Upper(notes)` or `notes + "!"`, is Character. `?`
wraps memo values at `SET MEMOWIDTH`.

Numbers keep a display width and decimal count, so `?` formats them the way
dBase and Clipper do. `SET DECIMALS` and `SET FIXED` control the decimals
shown for calculated values.

### 6.4 Operators

| Kind | Operators |
|---|---|
| Arithmetic | `+` `-` `*` `/` `%` (modulus) `**` or `^` (power) |
| Assignment | `:=` `=` (in a statement) `+=` `-=` `*=` `/=` `%=` `^=` `**=` |
| Increment | `++` `--` (prefix and postfix) |
| Comparison | `=` `==` (exact) `!=` `<>` `#` `<` `<=` `>` `>=` |
| Substring | `$` (`"b" $ "abc"` is `.T.`) |
| Logical | `.AND.` `.OR.` `.NOT.` `!` |
| Alias | `->` (`customer->name`, `customer->(RecNo())`) |
| Send | `:` (`oError:description`) |
| Reference | `@` (pass a variable by reference) |
| Macro | `&` (see [Macros](#610-macros)) |

String `+` joins strings; `-` joins them and moves trailing spaces of the
left string to the end. Date `+`/`-` a number adds or subtracts days, and
date minus date gives the number of days between them.

`=` compares strings using `SET EXACT`. With `SET EXACT OFF` (the default),
`"abc" = "ab"` is true because only the length of the right-hand string is
compared. `==` always compares exactly.

### 6.5 Variables and scope

| Declaration | Scope and lifetime |
|---|---|
| `LOCAL x [:= value]` | Visible only in the declaring function; created on each call |
| `STATIC x [:= value]` | Visible in the function (or file, if declared at file level); keeps its value between calls |
| `PRIVATE x` | Visible in the declaring routine and everything it calls; released when the routine returns |
| `PUBLIC x` | Visible everywhere until released |
| `PARAMETERS a, b` | Declares private parameters (dBase style) |
| `MEMVAR x` | Declares that `x` refers to a private/public variable |
| `FIELD x [IN alias]` | Declares that `x` refers to a database field |
| `DECLARE a[n]` / `DIMENSION a[n]` | Creates a private array |

Assigning to an undeclared name creates a private variable. When a name could
be a field or a memory variable, the field in the current work area wins. Use
`M->name` to mean the memory variable, and `FIELD->name` or `alias->name` for
the field.

### 6.6 Functions and procedures

```
FUNCTION Area(nWidth, nHeight)
   LOCAL nArea := nWidth * nHeight
RETURN nArea

PROCEDURE Greet(cName)
   ? "Hello,", cName
RETURN

STATIC FUNCTION Helper()   // visible only in this file
RETURN NIL
```

- Missing arguments are `NIL`. `PCount()` returns how many were passed.
- Arguments are passed by value, except arrays and hashes, which are shared.
  Pass `@var` to pass by reference.
- `DO proc WITH a, b` passes variables by reference.
- `PValue(n)` returns the nth argument. `hb_AParams()` returns all of them as
  an array.

### 6.7 Control structures

```
IF cond
ELSEIF cond
ELSE
ENDIF                       // or END

DO WHILE cond
   LOOP                     // next iteration
   EXIT                     // leave the loop
ENDDO

FOR i := 1 TO 10 [STEP 2]
NEXT

FOR EACH x IN aArray        // also hashes and strings
NEXT

DO CASE
CASE cond
OTHERWISE
ENDCASE

SWITCH expr
CASE 1
   ...
   EXIT                     // without EXIT, execution falls through
OTHERWISE
ENDSWITCH

BEGIN SEQUENCE [WITH bErrorHandler]
   BREAK [value]
RECOVER [USING var]
END [SEQUENCE]

TRY
   ...
CATCH [oErr]
FINALLY
END
```

### 6.8 Arrays

```
LOCAL a := {10, 20, 30}
LOCAL m[3, 2]               // 3 x 2 array of NIL
? a[1], Len(a)              // arrays are 1-based
AAdd(a, 40)
ASort(a, , , {|x, y| x > y})
? AScan(a, {|n| n == 20})
```

See [Array functions](#arrays) for the full list.

### 6.9 Hashes

```
LOCAL h := {"name" => "Ann", "age" => 30}
h["city"] := "Boston"
? h["name"], Len(h)
? hb_HHasKey(h, "age")
FOR EACH x IN hb_HKeys(h)
   ? x, h[x]
NEXT
```

### 6.10 Macros

| Form | Meaning |
|---|---|
| `&cVar` | Replaced by the contents of the character variable `cVar` |
| `&cVar.ext` | Macro followed by text; the `.` ends the macro name |
| `name&cVar` | Name built from text plus a macro, e.g. `opt&sub` is `OPT1` when `sub` is `"1"`; works on either side of `=` |
| `&(expr)` | Evaluates the character expression and compiles the result |
| `&cCmd` (as a statement) | Runs a whole command, e.g. `cCmd := "LIST"` |
| `&cFunc(args)` / `&cFunc.(args)` | Calls the function whose name is in `cFunc` |

```
cField := "name"
? &cField                 // value of the field NAME
cFile := "cust"
USE &cFile
? &("Upper('abc')")
```

Macros see PRIVATE and PUBLIC variables only, not LOCAL or STATIC ones, as
in Clipper.

```
PRIVATE cFunc := "Upper"
? &cFunc("abc")           // ABC
```

Macros are also expanded inside `TEXT ... ENDTEXT` blocks:

```
TEXT
Dear &cName.,
ENDTEXT
```

### 6.11 Code blocks

```
bAdd := {|a, b| a + b}
? Eval(bAdd, 2, 3)        // 5
AEval(aItems, {|x, i| QOut(i, x)})
```

Code blocks can use the local variables of the function that created them
(detached locals), even after that function has returned.

### 6.12 Date literals and the century

`0dYYYYMMDD` is a date literal. `CToD()` reads dates using `SET DATE` and
`SET EPOCH`; with the default epoch of 1950, two-digit years 50–99 are 19xx
and 00–49 are 20xx.

---

## 7. The preprocessor

Nantucket has a complete Clipper preprocessor. Every dBase/Clipper command is
defined as a rule in the built-in `std.ch`, so you can add commands of your
own in the same way.

| Directive | Purpose |
|---|---|
| `#define NAME value` | Define a constant (or a function-like macro) |
| `#undef NAME` | Remove a definition |
| `#ifdef` / `#ifndef` / `#else` / `#endif` | Conditional compilation |
| `#include "file.ch"` | Include a header (searched on disk first, then the built-in headers) |
| `#command` / `#xcommand` | Define a command (`#command` allows 4-letter abbreviations) |
| `#translate` / `#xtranslate` | Define a translation that can appear anywhere in a line |

Example:

```
#command SAY HELLO TO <name> => QOut( "Hello, " + <name> )

SAY HELLO TO "world"
```

Match markers follow Clipper: `<x>` (regular), `<x,...>` (list),
`<x:A,B>` (restricted), `<*x*>` (wild), `<(x)>` (extended). Result markers:
`<x>`, `#<x>` (dumb stringify), `<"x">` (normal stringify), `<(x)>` (smart
stringify), `<{x}>` (blockify), `<.x.>` (logify). Optional clauses use `[ ]`.

---

## 8. Command reference

Square brackets mark optional parts. `<scope>` is any of `ALL`, `NEXT <n>`,
`RECORD <n>` and `REST`, and `FOR <cond>` / `WHILE <cond>` can be added to
any command that takes a scope.

### 8.1 Output

| Command | Description |
|---|---|
| `? [list]` | Start a new line and print the expressions |
| `?? [list]` | Print without starting a new line |
| `EJECT` | Form feed to the printer |
| `TEXT ... ENDTEXT` | Print a block of text (with macro expansion) |

### 8.2 Screen

| Command | Description |
|---|---|
| `CLS` / `CLEAR SCREEN` | Clear the screen |
| `CLEAR` | Clear the screen and pending GETs |
| `@ r, c` | Clear from (r, c) to the end of the row |
| `@ r, c CLEAR [TO r2, c2]` | Clear a region |
| `@ r, c SAY expr [PICTURE p] [COLOR c]` | Display at a position |
| `@ r, c GET var [PICTURE p] [VALID cond] [WHEN cond] [RANGE lo, hi] [COLOR c]` | Create an entry field |
| `@ r, c SAY ... GET ...` | Both in one command |
| `@ t, l TO b, r [DOUBLE] [COLOR c]` | Draw a single or double box |
| `@ t, l, b, r BOX cString [COLOR c]` | Draw a box with custom characters |
| `@ r, c PROMPT cText [MESSAGE cMsg]` | Define a menu item |
| `MENU TO var` | Run the menu defined by `@ ... PROMPT` |
| `READ [SAVE]` | Edit the pending GETs (`SAVE` keeps them afterwards) |
| `CLEAR GETS` | Discard pending GETs |

### 8.3 Interaction

| Command | Description |
|---|---|
| `WAIT [prompt] [TO var]` | Wait for a key |
| `ACCEPT [prompt] TO var` | Read a line of text |
| `INPUT [prompt] TO var` | Read and evaluate an expression |
| `KEYBOARD cKeys` | Put keys in the keyboard buffer |
| `CLEAR TYPEAHEAD` | Empty the keyboard buffer |
| `SET KEY n TO proc` | Call `proc` when key `n` is pressed |
| `SET FUNCTION n TO cText` | Make function key `n` type `cText` |
| `HELP` | Show the command summary |

### 8.4 Program flow

| Command | Description |
|---|---|
| `DO proc [WITH args]` | Run a procedure or program |
| `RETURN [value]` | Return from a routine |
| `QUIT` | End the program and Nantucket |
| `CANCEL` / `RETURN TO MASTER` | Stop the program and return to the dot prompt |
| `RUN cmd` / `! cmd` | Run a shell command |
| `SET PROCEDURE TO [file] [ADDITIVE]` | Load a procedure file |
| `CLOSE PROCEDURE` | Unload procedure files |
| `COMPILE file` | Compile-check a program |

### 8.5 Memory variables

| Command | Description |
|---|---|
| `STORE value TO v1 [, v2...]` | Assign a value to one or more variables |
| `RELEASE v1 [, v2...]` | Release variables |
| `RELEASE ALL [LIKE skel \| EXCEPT skel]` | Release private variables matching a pattern (`*`, `?`) |
| `SAVE TO file [ALL [LIKE \| EXCEPT skel]]` | Save variables to a `.mem` file |
| `RESTORE FROM file [ADDITIVE]` | Load variables from a `.mem` file |
| `DISPLAY MEMORY` / `LIST MEMORY` | Show all memory variables |
| `CLEAR MEMORY` | Release all memory variables |
| `CLEAR ALL` | Close tables, release variables and GETs, close the alternate file |
| `DEFAULT v TO value [, ...]` | Assign a value if the variable is `NIL` |

### 8.6 Tables and work areas

| Command | Description |
|---|---|
| `USE [file] [ALIAS a] [NEW] [EXCLUSIVE \| SHARED] [READONLY] [INDEX i1, ...]` | Open a table (no file: close the current one) |
| `SELECT n \| alias` | Change the current work area |
| `CLOSE [alias]` | Close the current (or named) table |
| `CLOSE DATABASES` | Close all tables |
| `CLOSE INDEXES` | Close the indexes of the current table |
| `CLOSE ALL` | Close tables, alternate and printer files |
| `CLOSE ALTERNATE` | Close the alternate file |
| `CREATE file` | Design a new table interactively |
| `CREATE file FROM xfile [NEW] [ALIAS a]` | Create a table from a structure-extended table |
| `MODIFY STRUCTURE` | Change the structure of the current table interactively |
| `COPY STRUCTURE TO file [FIELDS list]` | Create an empty copy of the structure |
| `COPY STRUCTURE EXTENDED TO file` | Write the structure as records |
| `DISPLAY STRUCTURE` / `LIST STRUCTURE` | Show fields, record count and indexes |
| `DISPLAY STATUS` / `LIST STATUS` | Show open tables, indexes and settings |
| `COMMIT` | Write changes to disk |
| `UNLOCK [ALL]` | Release record and file locks. See [Sharing tables](#sharing-tables-between-users) |

### 8.7 Moving the record pointer

| Command | Description |
|---|---|
| `GO[TO] n` / `GO TOP` / `GO BOTTOM` | Go to a record, the first or the last |
| `SKIP [n] [ALIAS a]` | Move forward or backward |
| `SEEK expr [SOFTSEEK] [LAST]` | Search the controlling index |
| `FIND text` | `SEEK` with a literal string |
| `LOCATE [FOR cond] [WHILE cond] [scope]` | Find the first matching record |
| `CONTINUE` | Find the next record matching the last `LOCATE` |

### 8.8 Changing data

| Command | Description |
|---|---|
| `APPEND BLANK` | Add an empty record |
| `APPEND` | Add records with a full-screen form |
| `APPEND FROM file [FIELDS list] [scope] [SDF \| DELIMITED [WITH c \| BLANK \| TAB \| PIPE]]` | Import records from a table or text file |
| `REPLACE f1 WITH x1 [, f2 WITH x2] [scope]` | Change fields |
| `DELETE [scope]` | Mark records for deletion |
| `RECALL [scope]` | Unmark deleted records |
| `PACK` | Remove deleted records permanently |
| `ZAP` | Remove all records |
| `EDIT` / `CHANGE` | Edit records with a full-screen form |
| `BROWSE [FIELDS list]` | Edit records in a scrolling table |

### 8.9 Querying

| Command | Description |
|---|---|
| `LIST [exprs] [OFF] [scope] [TO PRINTER \| TO FILE f]` | List records (all by default) |
| `DISPLAY [exprs] [OFF] [scope] [TO PRINTER \| TO FILE f]` | Show the current record, or the scope given |
| `COUNT [TO var] [scope]` | Count records |
| `SUM [exprs] [TO vars] [scope]` | Add up numeric expressions |
| `AVERAGE [exprs] [TO vars] [scope]` | Average numeric expressions |
| `TOTAL ON key TO file [FIELDS list] [scope]` | Write subtotals for each key value |

`OFF` hides the record number. With `SET HEADING ON`, `LIST` and `DISPLAY`
print column headings.

### 8.10 Indexes, filters and relations

| Command | Description |
|---|---|
| `INDEX ON key TO file [FOR cond] [UNIQUE] [DESCENDING]` | Create an NTX index |
| `SET INDEX TO [i1, i2, ...] [ADDITIVE]` | Open indexes (no list: close them) |
| `SET ORDER TO [n]` | Choose the controlling index (0 = natural order) |
| `REINDEX` | Rebuild the open indexes |
| `SET FILTER TO [cond]` | Hide records that don't match |
| `SET RELATION TO key INTO alias [, ...] [ADDITIVE]` | Link a child table |
| `SET RELATION TO` | Remove relations |

### 8.11 Copying and sorting

| Command | Description |
|---|---|
| `COPY TO file [FIELDS list] [scope] [SDF \| DELIMITED [WITH c \| BLANK \| TAB \| PIPE]]` | Copy records to a table or text file |
| `SORT TO file ON f1 [/A /D /C], ... [scope]` | Write a sorted copy |
| `COPY FILE src TO dest` | Copy a file |

### 8.12 Files

| Command | Description |
|---|---|
| `DIR [skeleton]` / `DIRECTORY [skeleton]` | List tables, or files matching the skeleton |
| `TYPE file [TO PRINTER]` | Show a text file |
| `DELETE FILE file` / `ERASE file` | Delete a file |
| `RENAME old TO new` | Rename a file |
| `MODIFY COMMAND [file]` | Edit a program |
| `MODIFY FILE file` | Edit any text file |

---

## 9. SET reference

Each `SET` command changes a setting that `Set(nSet [, newValue])` can also
read and change. The `_SET_*` constants come from `set.ch`, which is always
included.

### On/off settings

All accept `ON`, `OFF`, `&macro` or `(logical expression)`.

| Command | Default | Set() constant | Effect |
|---|---|---|---|
| `SET ALTERNATE` | OFF | `_SET_ALTERNATE` | Echo console output to the alternate file |
| `SET BELL` | OFF | `_SET_BELL` | Beep during `READ` when a GET is filled or a key, date, `RANGE` or `VALID` is rejected |
| `SET CENTURY` | OFF | `_SET_CENTURY` | Show four-digit years |
| `SET CONFIRM` | OFF | `_SET_CONFIRM` | Require Enter to leave a full GET |
| `SET CONSOLE` | ON | `_SET_CONSOLE` | Show `?` output on screen |
| `SET CURSOR` | ON | `_SET_CURSOR` | Show the cursor |
| `SET DELETED` | OFF | `_SET_DELETED` | Hide records marked for deletion |
| `SET DELIMITERS` | OFF | `_SET_DELIMITERS` | Show delimiters around GETs |
| `SET ECHO` | OFF | `_SET_ECHO` | Show each program line as it runs. See [Tracing](#tracing-a-program) |
| `SET ESCAPE` | ON | `_SET_ESCAPE` | Allow Esc to end `READ`; when OFF, Esc is ignored |
| `SET EXACT` | OFF | `_SET_EXACT` | Exact string comparison with `=` |
| `SET EXCLUSIVE` | ON | `_SET_EXCLUSIVE` | ON: `USE` opens tables for this session alone; OFF: shared |
| `SET FIXED` | OFF | `_SET_FIXED` | Always show `SET DECIMALS` decimals |
| `SET HEADING` | ON | `_SET_HEADING` | Column headings in `LIST`/`DISPLAY` |
| `SET INSERT` | OFF | `_SET_INSERT` | Start GETs in insert mode |
| `SET INTENSITY` | ON | `_SET_INTENSITY` | Show GETs in the enhanced/unselected colours; when OFF they use the standard colour |
| `SET PRINTER` / `SET PRINT` | OFF | `_SET_PRINTER` | Echo console output to the printer file |
| `SET SAFETY` | ON | `_SET_SAFETY` | Ask before overwriting files |
| `SET SCOREBOARD` | ON | `_SET_SCOREBOARD` | Show "Ins" and READ messages (invalid date, `RANGE` limits) on row 0 |
| `SET SOFTSEEK` | OFF | `_SET_SOFTSEEK` | A failed `SEEK` stops at the next higher key |
| `SET STATUS` | OFF (ON at the dot prompt) | `_SET_STATUS` | Status bar on the bottom line of the dot prompt |
| `SET TALK` | OFF | `_SET_TALK` | Accepted for compatibility |
| `SET UNIQUE` | OFF | `_SET_UNIQUE` | New indexes keep only the first record for each key |
| `SET WRAP` | OFF | `_SET_WRAP` | Menu bars wrap from the last item to the first |

`SET TRUSTED ON` lets index keys and report and label forms run any
expression (see [Expressions in data files](#expressions-in-data-files));
it has no `Set()` constant.

`SET STEP ON` pauses before each program line (see
[Tracing](#tracing-a-program)). `SET DEBUG`, `SET DOHISTORY` and `SET MENU`
are accepted and ignored.

### Value settings

| Command | Default | Set() constant |
|---|---|---|
| `SET ALTERNATE TO [file] [ADDITIVE]` | none | `_SET_ALTFILE` |
| `SET COLOR TO [spec]` | from the theme | `_SET_COLOR` |
| `SET THEME TO [name]` | startup theme | (see `SetTheme()`) |
| `SET DATE [TO] format` | AMERICAN (`mm/dd/yy`) | `_SET_DATEFORMAT` |
| `SET DATE FORMAT [TO] "spec"` | | `_SET_DATEFORMAT` |
| `SET DECIMALS TO [n]` | 2 | `_SET_DECIMALS` |
| `SET DEFAULT TO [dir]` | none | `_SET_DEFAULT` |
| `SET DELIMITERS TO [c \| DEFAULT]` | `::` | `_SET_DELIMCHARS` |
| `SET DEVICE TO SCREEN \| PRINTER` | SCREEN | `_SET_DEVICE` |
| `SET EPOCH TO n` | 1950 | `_SET_EPOCH` |
| `SET MARGIN TO [n]` | 0 | `_SET_MARGIN` (left margin for printer output) |
| `SET MEMOWIDTH TO n` | 50 | `_SET_MEMOWIDTH` (wrap width for memos in `?`, `LIST`, `DISPLAY`) |
| `SET MESSAGE TO [row] [CENTER]` | 0 (off) | `_SET_MESSAGE`, `_SET_MCENTER` (row and centring of menu messages) |
| `SET PATH TO [dirs]` | none | `_SET_PATH` |
| `SET PRINTER TO [file] [ADDITIVE]` | none (`nantucket.prn`) | `_SET_PRINTFILE` |
| `SET TYPEAHEAD TO n` | 50 | `_SET_TYPEAHEAD` (size of the keyboard buffer) |
| `SET CODEPAGE TO [name]` | AUTO | (see [Code pages](#code-pages); `__SetCodePage()`) |

### Date formats

| `SET DATE` | Format |
|---|---|
| `AMERICAN`, `MDY` | mm/dd/yy |
| `ANSI` | yy.mm.dd |
| `BRITISH`, `FRENCH`, `DMY` | dd/mm/yy |
| `GERMAN` | dd.mm.yy |
| `ITALIAN` | dd-mm-yy |
| `JAPANESE`, `YMD` | yy/mm/dd |
| `USA` | mm-dd-yy |

With `SET CENTURY ON` the year has four digits.

---

## 10. Colours and themes

### Themes

A theme sets the default colours. Three themes are built in:

| Theme | Default `SET COLOR` | Look |
|---|---|---|
| `color` | `W+/B,W+/G,B,B,W+/G` | White on blue, GETs in white on green |
| `classic` | `W/N,N/W,N,N,N/W` | White on black, the plain Clipper default |
| `mono` | `W/N,N/W,N,N,N/W` | Black and white only |

The theme is chosen at startup from:

1. the `-theme <name>` option, then
2. the `NANTUCKET_THEME` environment variable, then
3. the terminal: `mono` if `NO_COLOR` is set or the terminal supports fewer
   than 8 colours, otherwise `color`.

`colour`, `bw` and `monochrome` are accepted as aliases.

In the `mono` theme every colour is shown as normal, bright or reverse text,
so programs written for colour stay readable:

- Bright foregrounds (`W+`, `GR+`, ...) become bold white on black.
- A dark foreground on a lighter background (`N/W`, `N/BG`) becomes reverse
  video.
- Matching foreground and background (`N/N`) stays hidden.

A theme also sets the colours of the built-in screens: the program editor,
`BROWSE`/`EDIT` headers, `Alert()` boxes and the dot prompt status bar.
`SET COLOR` does not change these.

Colours are drawn with the classic VGA palette (blue is `#0000AA`, white is
`#FFFFFF`, and so on), so they look the same whatever colour theme the
terminal uses. To use the terminal's own 16 colours instead, set
`NANTUCKET_PALETTE=terminal`. The `mono` theme always uses the terminal's
colours.

### SET COLOR

```
SET COLOR TO <standard>, <enhanced>, <border>, <background>, <unselected>
```

| Slot | Used for |
|---|---|
| Standard | `?`, `@ ... SAY`, normal text |
| Enhanced | The active GET, menu and browse highlight |
| Border | Screen border (accepted, not shown) |
| Background | Accepted for compatibility |
| Unselected | GETs that are not being edited |

Each slot is `foreground/background`:

| Code | Colour | Code | Colour |
|---|---|---|---|
| `N` | Black | `N+` | Grey |
| `B` | Blue | `B+` | Bright blue |
| `G` | Green | `G+` | Bright green |
| `BG` | Cyan | `BG+` | Bright cyan |
| `R` | Red | `R+` | Bright red |
| `RB` | Magenta | `RB+` | Bright magenta |
| `GR` | Brown | `GR+` | Yellow |
| `W` | White (light grey) | `W+` | Bright white |

Other codes: `U` underline, `I` inverse, `X` blank, `*` blinking. Numbers
0–15 can be used instead of letters.

How SET COLOR and themes work together:

```
SET COLOR TO GR+/B, W+/R   && override; slots left out keep the theme's colours
SET COLOR TO               && back to the theme's colours
SET THEME TO mono          && switch theme: resets colours and clears the screen
SET THEME TO               && back to the startup theme
? SetColor()               && current colour string
? SetTheme(), IsColor()    && theme name; IsColor() is .F. in mono
```

`COLOR` clauses on `@ ... SAY`, `@ ... GET` and `@ ... BOX` override the
colour for that one command.

---

## 11. Screen, keyboard and the GET system

### Coordinates

Rows and columns start at 0. `MaxRow()` and `MaxCol()` give the bottom row and
right column (normally 24 and 79, but the screen follows the terminal size).

### Windows

The screen can hold numbered windows on top of the full screen, which is
window 0. Each window has a frame whose top line is a title bar showing
its number and title, with three controls on the right:

```
╔═ 1: Customers ═══════════════[_][↑][x]╗
║                                       ║
╚═══════════════════════════════════════◢
```

`[_]` minimises, `[↑]` maximises (`[↕]` restores) and `[x]` closes. Drag
the title bar with the mouse to move a window, and drag the `◢` corner to
resize it. Clicking a window brings it to the front, and clicking a
minimised window's tab (along the bottom of the screen) restores it. The
window in front has a double line frame. The mouse is only turned on while
windows are open, so the terminal's own copy and paste works the rest of
the time.

Windows are **sizable** by default: the user can resize and maximise them.
Open a window `FIXED` (or pass `.F.` as `lSizable`) to stop that; it then
has only `[_]` and `[x]`. Programs can still resize any window with
`WSize()`.

**Selecting a window.** Opening a window selects it. While a window is
selected, every screen command works inside it: `?`, `@ ... SAY/GET`,
`READ`, `CLEAR`, `Scroll()`, `DispBox()`, `SaveScreen()`/`RestScreen()`,
`Row()`, `Col()`, `SetPos()`. Coordinates are relative to the inside of the
window, and `MaxRow()`/`MaxCol()` give its inside size, so code written for
the full screen runs unchanged in a window. Text past the edge is clipped.
`SET WINDOW TO <n>` selects another window and brings it to the front;
`SET WINDOW TO` (or `SET WINDOW TO 0`) goes back to the full screen.

Each window keeps its own `SET COLOR`: `COLOR` when it is opened sets it,
and selecting a window brings back the colours it last used.

```
WINDOW OPEN 2, 5 TO 12, 45 TITLE "Customers" COLOR "N/W" TO nCust
? "Inside the window"
@ 3, 2 SAY "Name" GET cName
READ
SET WINDOW TO                 && back to the full screen
WINDOW CLOSE nCust
```

| Command | Function | Does |
|---|---|---|
| `WINDOW OPEN t, l TO b, r [TITLE c] [COLOR c] [FIXED] [TO var]` | `WOpen(t, l, b, r [, cTitle] [, cColor] [, lSizable])` → n | Open and select a window. The frame is drawn on the given screen coordinates |
| `SET WINDOW TO [n]` | `WSelect([n])` → nOld | Select a window (0: full screen). No argument: report the current one |
| `WINDOW CLOSE [n]` | `WClose([n])` | Close a window (default: the current one) |
| `WINDOW CLOSE ALL` | `WAClose()` | Close every window |
| `WINDOW MOVE n TO t, l` | `WMove(t, l [, n])` | Move a window's frame |
| `WINDOW SIZE n TO rows, cols` | `WSize(rows, cols [, n])` | Set a window's frame size |
| `WINDOW MINIMIZE [n]` | `WMinimize([n])` | Minimise to a tab |
| `WINDOW MAXIMIZE [n]` | `WMaximize([n])` | Fill the screen (above the status bar) |
| `WINDOW RESTORE [n]` | `WRestore([n])` | Back to normal (a minimised window that was maximised goes back to maximised) |
| | `WRow([n])`, `WCol([n])`, `WLastRow([n])`, `WLastCol([n])` | Frame position: top, left, bottom, right |
| | `WState([n])` | 0 normal, 1 minimised, 2 maximised |
| | `WNum()` | Highest open window number (0 when none) |
| | `WList()` | Array of open window numbers |
| | `WTitle([cTitle] [, n])` → cOld | Get or set the title |
| | `WSizable([lSizable] [, n])` → lOld | Get or set whether the user can resize it |

**Checks.** These raise a runtime error (subsystem `TERM`) rather than
failing quietly:

- a window that is smaller than 3 by 3 or does not fit on the screen
  (open, move or size);
- a window number that is not open;
- closing, moving or titling window 0;
- maximising a window that is not sizable, or moving or sizing a maximised
  one;
- more than 255 windows;
- opening a window without the full screen console (`-stream`, scripts and
  `nantucket test`). `SET WINDOW TO 0` always works.

Output to a minimised window still goes to it; it shows when restored.
When the terminal is resized, windows are moved (and if need be shrunk) to
stay on screen. A window closed with the mouse while selected hands output
to the window now in front.

The dot prompt's `HELP` uses a window: it stays open while you carry on
typing, scrolls with the mouse wheel, and shows more when made bigger.

**Without a mouse.** At the dot prompt, F6 enters window mode, with a key
summary on the bottom line:

| Key | Action |
|---|---|
| Arrows | Move the front window |
| Shift + arrows | Resize it (sizable windows) |
| Tab / Shift-Tab | Bring the next / previous window to the front (restoring it if minimised) |
| M | Minimise |
| Z | Maximise or restore |
| X, Del | Close |
| Enter, Esc | Leave window mode |

F1 opens or closes `HELP`, and Esc on an empty command line closes the
`HELP` or `VERSION FULL` window in front.

### GETs and READ

```
LOCAL cName := Space(20), nAge := 0, dBorn := CToD("")
@ 5, 10 SAY "Name" GET cName PICTURE "@!"
@ 6, 10 SAY "Age " GET nAge  PICTURE "999" RANGE 0, 120
@ 7, 10 SAY "Born" GET dBorn VALID !Empty(dBorn)
READ
IF LastKey() == K_ESC
   ? "Cancelled"
ENDIF
```

| Key in READ | Action |
|---|---|
| Enter, Down, Tab | Next GET (leaves `READ` after the last one) |
| Up, Shift-Tab | Previous GET |
| PgUp, PgDn, Ctrl-W | End `READ` and keep the changes |
| Esc | End `READ` (changes to the current GET are lost). Ignored with `SET ESCAPE OFF` |
| Ctrl-Home | First GET |
| Left, Right, Home, End | Move within the GET |
| Ins | Toggle insert and overtype |
| Del, Backspace | Delete a character |
| Ctrl-Y | Clear the rest of the GET |
| Ctrl-T | Delete word |

- With `SET SCOREBOARD ON` (the default), row 0 shows "Ins" in insert mode
  and messages such as "Invalid date" or "Range: 1 - 120". Row 0 is restored
  when `READ` ends.
- `SET BELL ON` beeps when a GET fills up or input is rejected.
- `SET INTENSITY OFF` shows GETs in the standard colour.
- `VALID cond` is checked when leaving a GET; the cursor stays until it is
  true.
- `WHEN cond` skips the GET unless the condition is true.
- `RANGE lo, hi` checks numeric and date values.
- `ReadVar()`, `Updated()`, `ReadExit()`, `ReadInsert()` and `ReadKill()`
  work as in Clipper.

### PICTURE

A picture is `"@functions template"`, or just a template.

**Functions**

| Function | Effect |
|---|---|
| `!` | Convert letters to upper case |
| `(` | Show negative numbers in parentheses |
| `B` | Left-justify numbers |
| `C` | Show `CR` after positive numbers |
| `X` | Show `DB` after negative numbers |
| `E` | European format: swap `.` and `,` in numbers, use dd/mm/yy for dates |
| `R` | Template characters that aren't placeholders are inserted into the display only |
| `Z` | Show zero as blanks |
| `K` | Clear the GET when the first key typed is not a cursor key |
| `S<n>` | Scroll the GET in a window `n` characters wide |
| `Y` | Logical shown as Y/N |

**Template characters**

| Character | Allows / shows |
|---|---|
| `9` | Digit (and sign for numbers) |
| `#` | Digit, space or sign |
| `A` | Letter |
| `N` | Letter or digit |
| `X` | Any character |
| `L` | Logical T/F |
| `Y` | Logical Y/N |
| `!` | Any character, converted to upper case |
| `.` | Decimal point position |
| `,` | Thousands separator |
| `$`, `*` | Fill leading blanks with `$` or `*` |

`Transform(value, picture)` formats a value with a picture without a GET.

### Menus

```
@ 5, 10 PROMPT "Add"    MESSAGE "Add a record"
@ 6, 10 PROMPT "Browse" MESSAGE "Browse the table"
@ 7, 10 PROMPT "Quit"
MENU TO nChoice           // 0 if Esc was pressed
```

Arrows move the highlight, Enter chooses, Home/End jump to the first/last
item, and the first letter of an item selects it. `SET MESSAGE TO row
[CENTER]` chooses the row where messages appear and whether they are
centred. `SET WRAP ON` lets the highlight wrap.

### AChoice()

```
AChoice(nTop, nLeft, nBottom, nRight, aItems,
        [lSelectable | aSelectable], [cUserFunction],
        [nInitialItem], [nWindowRow]) -> nChosen
```

Shows `aItems` as a scrolling list and returns the chosen item number, or 0
if the user cancelled. An empty string in `aItems` ends the list.

- **Selectable items:** pass `.F.` to make every item unselectable, or an
  array with one entry per item. Each entry is a logical or a condition
  string such as `"nStock > 0"`. Unselectable items are shown in the
  unselected colour (SET COLOR slot 5) and skipped by the cursor.
- **Without a user function:** arrows, PgUp/PgDn, Home/End and
  Ctrl-PgUp/Ctrl-PgDn move. A letter jumps to the next item starting with it.
  Enter chooses, and Esc, Left or Right return 0.
- **With a user function:** AChoice calls `cUserFunction(nMode, nCurrent,
  nRow)`, where `nRow` is the highlight's row within the window. It calls
  with `AC_HITTOP` or `AC_HITBOTTOM` when the cursor tries to move past
  either end, and with `AC_EXCEPT` for any key it doesn't handle itself,
  including Enter and Esc. It also calls with `AC_IDLE` while it waits for a
  key, and with `AC_NOITEM` if no item can be chosen. The function returns
  `AC_ABORT` (return 0), `AC_SELECT` (choose the current item), `AC_CONT`
  (carry on) or `AC_GOTO` (jump to the item starting with `LastKey()`).
  The constants are in `achoice.ch`.

```
#include "achoice.ch"
#include "inkey.ch"

n := AChoice(5, 10, 15, 30, aNames, aInStock, "PickKeys")

FUNCTION PickKeys(nMode, nCur, nRow)
   IF nMode == AC_EXCEPT
      DO CASE
      CASE LastKey() == K_ENTER
         RETURN AC_SELECT
      CASE LastKey() == K_ESC
         RETURN AC_ABORT
      ENDCASE
      RETURN AC_GOTO
   ENDIF
RETURN AC_CONT
```

### Other screen functions

`Alert()` shows a message box with
buttons, `DispBox()` draws boxes, and `SaveScreen()`/`RestScreen()` save and
restore regions. See [Screen functions](#screen-and-keyboard).

### Keys and Inkey()

`Inkey([nSeconds])` returns the next key code (0 if none; `Inkey(0)` waits
forever). `#include "inkey.ch"` for names such as `K_ENTER`, `K_ESC`, `K_UP`,
`K_F1`, `K_CTRL_W` and `K_ALT_C`.

### Printer and alternate output

- `SET PRINTER TO file` sends printer output to a file. `nantucket.prn` is
  used if no file is given.
- `SET DEVICE TO PRINTER` sends `@ ... SAY` to the printer file. Moving to a
  later row writes line breaks, and moving to an earlier row starts a new
  page.
- `SET MARGIN TO n` indents every printed line by `n` spaces.
- `PRow()` and `PCol()` give the print position (not counting the margin);
  `SetPrc(r, c)` sets it. `EJECT` resets it to the top of the page.
- `SET PRINTER ON` echoes `?` output to the printer file.
- `SET ALTERNATE TO file` and `SET ALTERNATE ON` echo console output to a
  text file.

---

## 12. Databases

### Tables (DBF)

Nantucket reads and writes dBase III / Clipper DBF files and reads dBase IV
and FoxPro tables.

| Type | Name | Width | Notes |
|---|---|---|---|
| `C` | Character | 1–65535 | |
| `N` | Numeric | 1–20 | Up to width − 2 decimals |
| `F` | Float | 1–20 | Read like Numeric |
| `D` | Date | 8 | Stored as YYYYMMDD |
| `L` | Logical | 1 | |
| `M` | Memo | 10 | Text stored in a `.dbt` file |

These FoxPro types can be read but not created: `I` (integer), `B`
(double), `Y` (currency), `T` (date-time, read as a date), and `G`/`P`
(general/picture, read as memo). Memo files can be dBase `.dbt` or FoxPro
`.fpt`.

Field names are up to 10 characters. Longer names in programs match on their
first 10 characters, as in dBase.

### Code pages

DBF files keep text as bytes in a code page: DOS programs such as dBase and
Clipper used an OEM code page (437 in the US, 850 in much of Europe),
Windows programs one such as 1252. Nantucket works in Unicode, so
character and memo fields are converted as they are read and written, and
index keys are kept in the table's code page so they sort and match like
keys other programs write.

- A table whose header names its code page (the language driver byte that
  FoxPro, Visual dBase and Harbour write) is always read in that code page.
- Other tables use `SET CODEPAGE TO <name>`: `437`, `850`, `852`, `865`,
  `866`, `1250`, `1251`, `1252`, `UTF8` or `AUTO`. `SET CODEPAGE TO` on
  its own goes back to `AUTO`. The environment variable
  `NANTUCKET_CODEPAGE` sets the starting value.
- `AUTO`, the default, reads text that is valid UTF-8 as it is (tables
  written by Nantucket 0.1) and anything else as code page 437, and writes
  code page 437.
- `UTF8` stores text unchanged. Other programs won't show accented
  letters correctly.
- New tables record their code page in the header. `COPY`, `SORT`, `TOTAL`
  and `MODIFY STRUCTURE` keep the source table's code page.
- Characters the code page doesn't have are stored as `?`.
- `dbCodePage()` returns the current table's code page (or the session's
  when no table is open).

Conversion happens only at the table: strings in memory are UTF-8, so
`Len()` and `SubStr()` count an accented letter as two characters.

### Creating a table

Interactively:

```
. CREATE people
```

opens the structure editor (Enter edit, Ctrl-N insert, Ctrl-U delete,
Ctrl-W save, Esc cancel). After saving, you are asked whether to add records
now.

In code:

```
dbCreate("people", { ;
   {"NAME",   "C", 20, 0}, ;
   {"AGE",    "N",  3, 0}, ;
   {"JOINED", "D",  8, 0}, ;
   {"ACTIVE", "L",  1, 0}, ;
   {"NOTES",  "M", 10, 0}  })
```

`dbStruct()` returns the structure in the same format. `#include
"dbstruct.ch"` for the `DBS_NAME`, `DBS_TYPE`, `DBS_LEN` and `DBS_DEC`
constants.

### Work areas and aliases

- There are 250 work areas. `USE ... NEW` opens a table in the next free
  area. `SELECT` changes the current area by number or alias.
- The alias defaults to the file name. Use `alias->field` to read another
  area's field and `alias->(expr)` to evaluate an expression there.
- `Select()`, `Alias()`, `Used()` and `dbSelectArea()` work with areas in
  code.

### Indexes (NTX)

Nantucket reads and writes Clipper NTX indexes. Keys can be character,
numeric or date expressions.

```
INDEX ON Upper(name) TO pname
INDEX ON DToS(joined) + name TO pjoined FOR active UNIQUE
SET INDEX TO pname, pjoined
SET ORDER TO 2
SEEK "SMITH"
IF Found()
   ...
ENDIF
```

- The first open index controls the order unless `SET ORDER` chooses another.
- All open indexes are updated when records change.
- `SET SOFTSEEK ON` (or `SEEK ... SOFTSEEK`) leaves the pointer on the next
  higher key when there is no match.
- `SEEK ... LAST` finds the last record with the key.
- `Descend()` gives keys that sort in reverse.
- `USE ... INDEX` and `SET INDEX TO` go to the first record in index order.
- Indexes are held in memory as blocks of sorted keys, so adding records to
  a large indexed table stays fast (100,000 records in under half a second).
- Index changes are written as a short change log at the end of the index
  file, so `COMMIT` and shared tables don't rewrite whole indexes. The index
  is written out whole (a plain NTX file, as Clipper and Harbour expect)
  when the table is closed or the log grows long.
- An index that may be missing records is rebuilt automatically when it is
  opened: one that wasn't saved after a change (Nantucket was killed, or
  the machine lost power), or one that missed records added while it
  wasn't open. At the dot prompt a message says so. This check works for
  indexes written by Nantucket 0.2 or later; indexes from other programs
  can't be checked.

### Expressions in data files

Index files keep their key and `FOR` expressions as text, and report and
label forms keep theirs, so a file from someone else could carry code (an
index key that deletes files, say). Before Nantucket runs an expression
from a file, it checks that it only reads data: fields, variables,
literals, operators, `IIF`, functions without side effects (`Upper()`,
`SubStr()`, `DToS()`, `Str()`, `Descend()` and the like) and functions
defined by the program that is running. Macros, assignments, code blocks
and functions that touch files, the screen or the shell are refused with
an error when the file is opened.

If you trust the file, `SET TRUSTED ON` and open it again.

### Sharing tables between users

Several people, or several copies of Nantucket, can use the same tables at
once, on one machine or over a shared network drive. Nantucket uses real
operating-system locks, so other Nantucket sessions see them.

**Opening.** `SET EXCLUSIVE` is ON by default, so `USE` opens a table for
this session alone. `USE ... SHARED` (or `SET EXCLUSIVE OFF`) lets others
open it too. If the open conflicts with another session (the table is open
`EXCLUSIVE` elsewhere, or you ask for `EXCLUSIVE` while others have it
open), `USE` doesn't fail with an error: it leaves the work area empty and
sets `NetErr()` to `.T.`. At the dot prompt a message says so.

```
USE customer SHARED NEW
IF NetErr()
   Alert("Customer file is in use; try again later")
   RETURN
ENDIF
```

**Locking.** On a shared table, lock a record before changing it.
Changing a record without a lock raises error 1022, "Lock required".

```
IF RLock()
   REPLACE balance WITH balance + nAmount
   UNLOCK
ELSE
   Alert("Someone else is editing this customer")
ENDIF
```

| Function | Does |
|---|---|
| `RLock()` / `Lock()` | Releases this area's other record locks, then locks the current record. `.F.` if another session has it |
| `dbRLock([nRec])` | Adds a lock on a record (default: the current one) without releasing others |
| `FLock()` | Locks the whole table, releasing record locks. `.F.` if anyone else holds a lock |
| `dbUnlock()` / `UNLOCK` | Writes the current record and releases this area's locks |
| `dbUnlockAll()` / `UNLOCK ALL` | The same for every work area |
| `dbRUnlock([nRec])` | Releases one record lock (or all of them) |
| `dbRLockList()` | Array of the record numbers this area has locked |
| `IsLocked([nRec])` | Is the record (or, with `FLock()`, the table) locked by this session? |
| `IsShared()` | Was the current table opened `SHARED`? |
| `NetErr([lNew])` | Did the last `USE` or `APPEND BLANK` fail because of another session? |

`APPEND BLANK` on a shared table adds the record and locks it, releasing
other record locks (unless the table is `FLock()`ed). `BROWSE` and `EDIT`
lock each record while it is saved, so they work on shared tables without
extra code; if someone else has the record locked they say "Record is in
use by another user" and leave it unchanged.

**Seeing other users' changes.** Records are read from disk each time you
move to one, so moving the pointer (`SKIP`, `GO`, `SEEK`, or `SKIP 0` to
stay put) shows the latest data. New records added by others appear in
`LastRec()` and navigation, and shared indexes are reloaded when another
session changes them.

**Needs exclusive use.** `PACK`, `ZAP` and `REINDEX` raise error 1023,
"Exclusive required", on a shared table.

**Limits.**
- On a shared table each index change is written to disk straight away,
  rewriting the whole index file, so adding many records to a large shared
  table is slower than with exclusive use. Use `FLock()` or `EXCLUSIVE` for
  big batch jobs.
- On Linux, locks belong to each open table, so two work areas in one
  session lock independently, like two users. On macOS and other Unix
  systems, locks belong to the whole session (so two work areas share
  them), and are dropped if the session opens and closes the same file
  some other way. Windows and macOS are not yet tested.
- The lock layout (record locks at byte 1,000,000,000 + record number)
  follows Clipper and Harbour on Linux, but working alongside Clipper or
  Harbour programs on the same tables has not been tested.

### Reports and labels

`REPORT FORM` and `LABEL FORM` print the current table using report
(`.frm`) and label (`.lbl`) forms. `CREATE REPORT` and `CREATE LABEL`
open full screen designers to make or change them (`MODIFY REPORT` and
`MODIFY LABEL` do the same), and `CREATE REPORT <file> FROM <fields>`
makes a simple report in one step:

```
USE customer INDEX state
CREATE REPORT custlist FROM name, state, balance
REPORT FORM custlist
```

See [COMMANDS.md](COMMANDS.md#create-report-modify-report). Forms are read
and written in the published Clipper layout, so forms made with dBase III
or Clipper work too, but they have only been tested with files in that
layout written by Nantucket; please report any form that doesn't print as
expected.

### Deleted records

`DELETE` marks records, `RECALL` unmarks them, and `PACK` removes them.
`SET DELETED ON` hides marked records from navigation and commands.
`Deleted()` tells you whether the current record is marked.

### Filters and relations

```
SET FILTER TO state == "MA" .AND. !Deleted()
SET RELATION TO custno INTO orders
```

A relation moves the child table to the record matching the key each time
the parent moves (by `SEEK` if the child has an index, otherwise by record
number).

### Import and export

- `COPY TO file` writes a DBF. Add `SDF` for fixed-width text or
  `DELIMITED` for comma-separated text with quoted strings. `WITH c` quotes
  with `c` instead of `"`; `WITH BLANK` separates with spaces and no quotes;
  `WITH TAB` and `WITH PIPE` separate with a tab or `|`.
- `APPEND FROM file` reads the same formats.
- `COPY STRUCTURE EXTENDED` and `CREATE FROM` convert between a table and its
  structure description.

### Interactive tools

| Command | Keys |
|---|---|
| `BROWSE` | Arrows/Tab move, PgUp/PgDn page, Ctrl-PgUp/Ctrl-PgDn top/bottom, Enter edit cell, Ctrl-N add record, Ctrl-U delete/recall, Esc or Ctrl-W exit |
| `EDIT` / `APPEND` | Up/Down move between fields, PgUp/PgDn previous/next record, Ctrl-N add, Ctrl-U delete/recall, Ctrl-W save, Esc exit |
| `MODIFY STRUCTURE` | Enter edit, Ctrl-N insert field, Ctrl-U delete field, Ctrl-W save, Esc cancel |

Memo fields open in the program editor.

---

## 13. Function reference

Arguments in brackets are optional. Function names are not case sensitive.
Functions starting with `hb_` are Harbour-compatible extensions.

### Strings

| Function | Returns |
|---|---|
| `AllTrim(c)` | `c` without leading or trailing spaces |
| `Asc(c)` | Code of the first character |
| `At(cFind, c)` | Position of `cFind` in `c` (0 if not found) |
| `Chr(n)` | Character with code `n` |
| `Descend(x)` | Value that sorts in reverse order (for index keys) |
| `Difference(c1, c2)` | 0–4: how alike two strings sound (`Soundex()`) |
| `Empty(x)` | `.T.` for blank strings, 0, empty dates, `.F.`, empty arrays and `NIL` |
| `HardCR(c)` | Soft carriage returns replaced with hard ones |
| `IsAlpha(c)` / `IsDigit(c)` / `IsLower(c)` / `IsUpper(c)` | Tests on the first character |
| `Left(c, n)` / `Right(c, n)` | First / last `n` characters |
| `Len(x)` | Length of a string, array or hash |
| `Lower(c)` / `Upper(c)` | Case conversion |
| `LTrim(c)` / `RTrim(c)` / `Trim(c)` | Remove leading / trailing spaces |
| `MemoLine(c, [nWidth], [nLine])` | One wrapped line of a memo |
| `MemoTran(c, [cHard], [cSoft])` | Replace line breaks |
| `MLCount(c, [nWidth])` | Number of wrapped lines |
| `MLPos(c, nWidth, nLine)` | Position where a wrapped line starts |
| `PadL(x, n, [cFill])` / `PadR(...)` / `PadC(...)` / `Pad(...)` | Pad to width `n` (left, right, centre) |
| `RAt(cFind, c)` | Position of the last `cFind` in `c` |
| `Replicate(c, n)` | `c` repeated `n` times |
| `Soundex(c)` | Four-character sound code |
| `Space(n)` | `n` spaces |
| `Str(n, [nWidth], [nDec])` | Number as a string |
| `StrTran(c, cFind, [cReplace], [nStart], [nCount])` | Replace occurrences |
| `StrZero(n, [nWidth], [nDec])` | Number with leading zeros |
| `Stuff(c, nStart, nDelete, cInsert)` | Delete and insert characters |
| `SubStr(c, nStart, [nLen])` | Part of a string |
| `Transform(x, cPicture)` | Value formatted with a picture |
| `Val(c)` | Number from a string |
| `hb_At(cFind, c, [nStart], [nEnd])` | `At()` with a start position |
| `hb_ATokens(c, [cDelim])` | Array of tokens |
| `hb_LeftEq(c, cPrefix)` | `.T.` if `c` starts with `cPrefix` |
| `hb_NToS(n)` | Number as a trimmed string |
| `hb_StrReplace(c, aFind, aReplace)` | Replace several substrings |
| `hb_TokenCount(c, [cDelim])` | Number of tokens |
| `hb_TokenGet(c, n, [cDelim])` | The nth token |
| `hb_ValToStr(x)` | Any value as a trimmed string |

### Numbers

| Function | Returns |
|---|---|
| `Abs(n)` | Absolute value |
| `ACos(n)` / `ASin(n)` / `ATan(n)` / `Cos(n)` / `Sin(n)` / `Tan(n)` | Trigonometry (radians) |
| `Ceiling(n)` / `Floor(n)` | Round up / down to a whole number |
| `Exp(n)` / `Log(n)` / `Log10(n)` / `Sqrt(n)` | Exponent, logarithms, square root |
| `Int(n)` | Whole-number part |
| `Max(x, y)` / `Min(x, y)` | Larger / smaller (numbers or dates) |
| `Mod(n, nDiv)` | dBase modulus |
| `Pi()` | π |
| `Rand([n])` / `hb_Random([n1], [n2])` | Random number from 0 to 1, 0 to `n1`, or `n1` to `n2` |
| `hb_RandomInt([n1], [n2])` | Random whole number from 1 to `n1`, or `n1` to `n2` |
| `Round(n, nDec)` | Rounded value |

### Dates and times

| Function | Returns |
|---|---|
| `AddMonth(d, [n])` | Date `n` months later (clamped to the end of the month) |
| `BoM(d)` / `EoM(d)` | First / last day of the month |
| `CDoW(d)` / `CMonth(d)` | Day / month name |
| `CToD(c)` | Date from a string in the `SET DATE` format |
| `Date()` | Today |
| `Day(d)` / `Month(d)` / `Year(d)` | Parts of a date |
| `DoW(d)` | Day of the week (1 = Sunday) |
| `DToC(d)` | Date as a string in the `SET DATE` format |
| `DToS(d)` | Date as `YYYYMMDD` |
| `ElapTime(cStart, cEnd)` | Time between two `hh:mm:ss` times |
| `Seconds()` | Seconds since midnight |
| `SToD(c)` / `hb_SToD(c)` | Date from `YYYYMMDD` |
| `Time()` | Current time as `hh:mm:ss` |
| `hb_Date(nYear, nMonth, nDay)` | Date from its parts |
| `hb_MilliSeconds()` | Milliseconds since 1970 |

### Arrays

| Function | Returns / does |
|---|---|
| `AAdd(a, x)` | Append `x` |
| `AClone(a)` | Deep copy |
| `ACopy(aSrc, aDest, [nStart], [nCount], [nDestPos])` | Copy elements |
| `ADel(a, n)` | Delete element `n` (the last becomes `NIL`) |
| `AEval(a, bBlock, [nStart], [nCount])` | Call `bBlock` with each element and its index |
| `AFill(a, x, [nStart], [nCount])` | Fill with a value |
| `AIns(a, n)` | Insert `NIL` at `n` (the last element is lost) |
| `Array(n1, [n2...])` | New array of `NIL` |
| `AScan(a, x \| bBlock, [nStart], [nCount])` | Index of the first match (0 if none) |
| `ASize(a, n)` | Grow or shrink |
| `ASort(a, [nStart], [nCount], [bOrder])` | Sort in place |
| `ATail(a)` | Last element |
| `hb_AScan(a, x, [nStart], [nCount], [lExact])` | `AScan()` with exact matching |
| `hb_AParams()` | The current function's arguments |

### Hashes

| Function | Returns / does |
|---|---|
| `Hash([k1, v1, ...])` / `hb_Hash(...)` | New hash |
| `HGet(h, k)` / `hb_HGet(h, k)` | Value for key `k` |
| `hb_HGetDef(h, k, xDefault)` | Value for `k`, or the default |
| `HSet(h, k, v)` / `hb_HSet(h, k, v)` | Set a key |
| `HDel(h, k)` / `hb_HDel(h, k)` | Delete a key |
| `HHasKey(h, k)` / `hb_HHasKey(h, k)` | `.T.` if the key exists |
| `HGetKeys(h)` / `hb_HKeys(h)` | Array of keys |
| `HGetValues(h)` / `hb_HValues(h)` | Array of values |
| `hb_HClone(h)` | Copy |

### Types and values

| Function | Returns |
|---|---|
| `ValType(x)` | Type letter of a value |
| `Type(cExpr)` | Type letter of an expression given as a string (`U` if undefined, `UE` on error) |
| `Eval(bBlock, [args...])` | Result of a code block |
| `hb_IsArray(x)`, `hb_IsBlock`, `hb_IsChar`, `hb_IsString`, `hb_IsDate`, `hb_IsHash`, `hb_IsLogical`, `hb_IsNil`, `hb_IsNumeric`, `hb_IsObject` | Type tests |
| `IsFunction(cName)` / `hb_IsFunction(cName)` | `.T.` if a function with this name exists |
| `FieldBlock(cField)` / `FieldWBlock(cField, nArea)` | Block that gets/sets a field |
| `MemVarBlock(cName)` | Block that gets/sets a memory variable |
| `hb_Default(@x, xDefault)` | Accepted for compatibility (use the `DEFAULT ... TO` command) |

### Screen and keyboard

| Function | Description |
|---|---|
| `AChoice(t, l, b, r, aItems, [aSelectable], [cUserFunc], [nInitial], [nWindowRow])` | Scrolling pick list; returns the chosen index or 0. See [AChoice()](#achoice) |
| `Alert(cMessage, [aButtons])` | Message box; returns the chosen button or 0. Use `;` in the message for new lines |
| `Col()` / `Row()` | Cursor position |
| `DevOut(x, [cColor])` / `DevOutPict(x, cPicture, [cColor])` / `DevPos(r, c)` | Output used by `@ ... SAY` (follows `SET DEVICE`) |
| `DispBegin()` / `DispEnd()` / `DispCount()` | Batch screen updates |
| `DispBox(t, l, b, r, [nType \| cChars], [cColor])` | Draw a box (1 = single, 2 = double) |
| `DispOut(x, [cColor])` / `DispOutAt(r, c, x, [cColor])` | Write to the screen |
| `Inkey([nSeconds])` | Key code |
| `AltD([nAction])` | Open the debugger; `AltD(0)` turns Alt-D off, `AltD(1)` on. See [Debugging](#debugging-a-program) |
| `IsColor()` / `IsColour()` | `.F.` in the mono theme |
| `LastKey()` / `NextKey()` | Last key read / next key waiting |
| `MaxCol()` / `MaxRow()` | Screen size |
| `MemoEdit([c], [t], [l], [b], [r], [lEdit])` | Edit text in a window; returns the text |
| `QOut([list])` / `QQOut([list])` | Same as `?` / `??` |
| `OutStd(...)` / `OutErr(...)` | Write to standard output / error |
| `SaveScreen([t], [l], [b], [r])` / `RestScreen([t], [l], [b], [r], cScreen)` | Save / restore a region |
| `Scroll([t], [l], [b], [r], [n])` | Scroll a region (0 clears it) |
| `SetColor([cSpec])` | Current colour string; sets a new one if given (surrounding quotes are ignored) |
| `SetCursor([n])` | Cursor shape (0 = off) |
| `SetKey(nKey, [bAction])` | Get or set a hot key |
| `SetPos(r, c)` | Move the cursor |
| `SetTheme([cName])` | Current theme name; switches theme if given (`""` = startup theme) |
| `hb_KeyPut(nKey)` | Put a key in the keyboard buffer |
| `Tone(nFreq, nDur)` | Beep |
| `Browse([t], [l], [b], [r], [aFields])` | Browse the current table |
| `dbEdit([t], [l], [b], [r], [aColumns], ...)` | Browse with column expressions |
| `PCol()` / `PRow()` / `SetPrc(r, c)` | Printer column and row / set them |

### The GET system

| Function | Description |
|---|---|
| `ReadModal(aGetList)` | Edit a list of GETs (used by `READ`) |
| `GetNew(...)` | Create a GET object |
| `ReadVar()` | Name of the GET being edited |
| `Updated()` | `.T.` if the last `READ` changed anything |
| `ReadExit([l])` | Whether Up/Down leave the first/last GET |
| `ReadInsert([l])` | Insert mode |
| `ReadKill([l])` | End the current `READ` |

### Database

| Function | Description |
|---|---|
| `dbUseArea([lNew], [cDriver], cFile, [cAlias], [lShared], [lReadOnly])` | Open a table |
| `dbCloseArea()` / `dbCloseAll()` | Close the current / all tables |
| `dbSelectArea(n \| cAlias)` | Change work area |
| `Select([cAlias])` | Number of the current (or named) area |
| `Alias([n])` | Alias of the current (or nth) area |
| `dbCodePage()` | Code page of the current table (see [Code pages](#code-pages)) |
| `Used()` | `.T.` if a table is open in the current area |
| `dbCreate(cFile, aStruct)` | Create a table |
| `dbStruct()` | Structure array |
| `dbAppend()` | Add a blank record |
| `dbDelete()` / `dbRecall()` / `Deleted()` | Mark / unmark / test deletion |
| `dbGoTo(n)` / `dbGoTop()` / `dbGoBottom()` / `dbSkip([n])` | Move the pointer |
| `dbSeek(x, [lSoft], [lLast])` | Search the controlling index |
| `dbEval(bBlock, [bFor], [bWhile], [nNext], [nRec], [lRest])` | Run a block for each record in scope |
| `dbCommit()` / `dbCommitAll()` | Write changes |
| `dbSetFilter(bCond, [cCond])` / `dbClearFilter()` / `dbFilter()` | Filters |
| `dbSetRelation(cAlias, bKey, [cKey])` / `dbClearRelation()` / `dbClearRel()` | Relations |
| `dbRelation(n)` / `dbRSelect(n)` | Key and child area of a relation |
| `dbCreateIndex(cFile, cKey, bKey, [lUnique])` | Create an index |
| `dbSetIndex(cFile)` / `dbClearIndex()` / `dbReindex()` / `dbSetOrder(n)` | Manage indexes |
| `IndexKey([n])` / `IndexOrd()` | Key expression / number of the controlling index |
| `OrdCount()`, `OrdKey()`, `OrdKeyCount()`, `OrdListAdd()`, `OrdListClear()`, `OrdName()`, `OrdNumber()`, `OrdSetFocus()` | Order (index) management |
| `BOF()` / `EOF()` / `Found()` | Pointer state |
| `RecNo()` / `RecCount()` / `LastRec()` | Record number / count |
| `RecSize()` / `Header()` | Record length / header size |
| `LUpdate()` | Date of last update |
| `FCount()` | Number of fields |
| `Field(n)` / `FieldName(n)` | Name of field `n` |
| `FieldPos(cName)` | Number of a field |
| `FieldGet(n)` / `FieldPut(n, x)` | Read / write a field by number |
| `FieldType(n)` / `FieldLen(n)` / `FieldDec(n)` | Field attributes |
| `AFields([aNames], [aTypes], [aLens], [aDecs])` | Fill arrays with field information |
| `RLock()` / `FLock()` / `dbRLock()` / `Lock()` | Lock a record or the table. See [Sharing tables](#sharing-tables-between-users) |
| `dbUnlock()` / `dbUnlockAll()` / `dbRUnlock()` / `dbRLockList()` | Release locks, list locked records |
| `IsLocked([nRec])` / `IsShared()` | Lock and open-mode checks |
| `NetErr([lNew])` | Did the last `USE` or `APPEND BLANK` fail because of another user? |

### Files and directories

| Function | Description |
|---|---|
| `FCreate(cFile, [nAttr])` | Create a file; returns a handle or -1 |
| `FOpen(cFile, [nMode])` | Open a file; returns a handle or -1 |
| `FClose(nHandle)` | Close |
| `FRead(nHandle, @cBuffer, nBytes)` | Read into a buffer; returns bytes read |
| `FReadStr(nHandle, nBytes)` | Read a string |
| `FWrite(nHandle, c, [nBytes])` | Write; returns bytes written |
| `FSeek(nHandle, nOffset, [nOrigin])` | Move the file position |
| `FError()` | Error code of the last file operation |
| `FErase(cFile)` / `FRename(cOld, cNew)` | Delete / rename |
| `File(cSkeleton)` / `hb_FileExists(cFile)` | `.T.` if a matching file exists |
| `hb_DirExists(cDir)` | `.T.` if the directory exists |
| `FileDate(cFile)` | File's modification date |
| `Directory([cSkeleton], [cAttr])` | Array of `{name, size, date, time, attr}` |
| `ADir([cSkeleton], [aNames], [aSizes], [aDates], [aTimes], [aAttrs])` | Fill arrays with directory entries; returns the count |
| `CurDir()` | Current directory |
| `DirChange(cDir)` / `MakeDir(cDir)` | Change / create a directory; 0 on success |
| `MemoRead(cFile)` / `hb_MemoRead(cFile)` | Whole file as a string |
| `MemoWrit(cFile, c)` / `hb_MemoWrit(cFile, c)` | Write a string to a file |
| `Bin2I()`, `Bin2L()`, `Bin2W()`, `I2Bin()`, `L2Bin()` | Binary integer conversion |

`#include "fileio.ch"` for `FO_READ`, `FO_WRITE`, `FO_READWRITE`, `FS_SET`,
`FS_RELATIVE` and `FS_END`.

### System and runtime

| Function | Description |
|---|---|
| `GetEnv(cName)` / `GetE(cName)` / `hb_GetEnv(cName)` | Environment variable |
| `OS()` | Operating system name |
| `Version()` | `"Nantucket 0.2"` |
| `hb_Compiler()` | Go version used to build Nantucket |
| `NetName()` | Host name |
| `hb_Shell(cCmd)` | Run a shell command |
| `ProcName([n])` / `ProcLine([n])` / `ProcFile([n])` | Name, line and file of the current (or a calling) routine |
| `PCount()` / `PValue(n)` | Argument count / value |
| `Set(nSet, [xValue])` | Read or change a setting |
| `SetMode()`, `SetBlink()`, `NoSnow()`, `ColorSelect()`, `Memory()`, `DiskSpace()`, `ErrorLevel()`, `FuncName()` | Accepted for compatibility (return fixed values) |

### Errors

| Function | Description |
|---|---|
| `ErrorBlock([bHandler])` | Get or set the error handler |
| `ErrorNew()` | New Error object |
| `DefError(oErr)` | The default error handler |
| `Break([x])` | Jump to the nearest `RECOVER` with value `x` |

---

## 14. Error handling

Runtime errors are Clipper Error objects. Unless handled, they stop the
program and print a report:

```
Error BASE/1001  Undefined function: FOO
Called from MAIN(12)
```

### Error object

| Property | Meaning |
|---|---|
| `:description` | Text of the error |
| `:subSystem` | `BASE`, `DBCMD`, `DBFNTX`, `TERM` or `COMPILER` |
| `:genCode` | General error code (see `error.ch`) |
| `:subCode` | Detailed error code |
| `:operation` | The operator, function or name involved |
| `:filename` | File involved, if any |
| `:args` | Arguments of the failed operation |
| `:severity` | Severity |
| `:canRetry`, `:canDefault`, `:canSubstitute` | What the handler may do |
| `:osCode` | Operating-system error code |
| `:cargo` | Free for your own use |
| `:tries` | Retry count |
| `:className` | `"ERROR"` |

### Catching errors

`BEGIN SEQUENCE WITH <block>` uses `<block>` as the error handler while the
sequence runs, and restores the previous handler afterwards. With
`{|e| Break(e)}`, any error jumps to `RECOVER` with the Error object:

```
BEGIN SEQUENCE WITH {|e| Break(e)}
   x := 1 / "a"
RECOVER USING oErr
   ? "Failed:", oErr:description
END SEQUENCE

TRY
   USE missing
CATCH oErr
   ? oErr:description
END

bOld := ErrorBlock({|e| MyHandler(e)})
```

When an error handler returns a value and the error has `:canSubstitute`
set, that value is used as the result of the failed operation.

Compile errors (syntax errors) are reported with the file and line when a
program is loaded or saved in the editor.

---

## 15. Testing programs

Nantucket has a built-in test runner for `.prg` code. A test can check
values with assertions, compare the program's output with an expected-output
file, or both.

```sh
nantucket test                      # every *_test.prg under the current directory
nantucket test tests                # every *_test.prg under tests/
nantucket test tests/calc_test.prg  # one file
nantucket test -run Tax             # only tests whose name contains "Tax"
nantucket test -update              # write or refresh the expected .out files
nantucket test -v                   # list each test and show each file's output
```

The runner exits with 0 when every test passes and 1 when any fails, so it
works in CI. `make test-prg` runs the examples in `examples/tests`.

To run a program called `test.prg`, use `nantucket test.prg`. Plain
`nantucket test` always starts the test runner.

### Test files

- Test files are named `<something>_test.prg`. Directories are searched
  recursively, skipping hidden directories.
- Each file runs in a fresh runtime with plain line-by-line output (as with
  `-stream`), with its own directory as the working directory. Tests can use
  data files kept beside them, but should delete any files they create.
- Every `FUNCTION` or `PROCEDURE` whose name starts with `Test` is a
  separate test, run in the order it appears. Statements before the first
  `FUNCTION`/`PROCEDURE` run once first, as setup; variables they create
  (even `PRIVATE` ones) are visible to every test.
- A file with no `Test` procedures is run as an ordinary program and counts
  as one test.
- A test fails if an assertion fails or a runtime error occurs. After an
  assertion fails, the test carries on, so you see every failure. A runtime
  error ends that test only; the next one still runs.

### Assertions

| Assertion | Passes when |
|---|---|
| `ASSERT <condition> [MESSAGE <text>]` | The condition is true. The message defaults to the condition's source text |
| `AssertTrue(lValue, [cMessage])` | The value is `.T.` |
| `AssertFalse(lValue, [cMessage])` | The value is `.F.` |
| `AssertEquals(xExpected, xActual, [cMessage])` | The values are exactly equal (`==`). Arrays and hashes are compared element by element |
| `AssertError(bBlock, [cMessage])` | Evaluating the block raises a runtime error. Returns the Error object so you can check it further |

Each returns `.T.` when it passes. A failure is reported with the file and
line:

```
FAIL  tests/calc_test.prg (0.01s)
      FAIL  TestTax
            calc_test.prg:12: with tax: expected 24, got 23.5
```

Outside the test runner, a failed assertion raises a runtime error
(`Assertion failed: ...`), so `ASSERT` can also guard ordinary programs.

### Expected output

If `<name>_test.out` exists beside the test file, the file's output must
match it. Differences are shown as a line diff, `-` for expected lines that
are missing and `+` for lines that were printed instead.

- Line ends are normalised, spaces at the ends of lines are ignored, and so
  are a leading blank line (from the first `?`) and blank lines at the end.
- Write the `.out` file by hand, or run with `-update` to save the current
  output. Check the result before committing it.
- With `-run`, output isn't compared when only some of a file's tests run.

### Input

If `<name>_test.in` exists, it is used as the keyboard: each line answers
one `ACCEPT`, `INPUT`, `WAIT`, `MENU TO` or `READ` field, in order. Prompts
appear in the output; the typed answers don't. Without an `.in` file, a
program that waits for input stops there.

### Example

`calc_test.prg`:

```
PUBLIC nRate := 0.2              // setup: runs once, before the tests

FUNCTION AddTax(nAmount)
RETURN nAmount * (1 + nRate)

PROCEDURE TestAddTax
   AssertEquals(120, AddTax(100))
   AssertEquals(0, AddTax(0), "zero stays zero")
RETURN

PROCEDURE TestBadInput
   LOCAL oErr := AssertError({|| AddTax("abc")})
   ASSERT oErr:subCode > 0
RETURN

PROCEDURE TestReport
   ? "Net:", 100
   ? "Gross:", AddTax(100)
RETURN
```

`calc_test.out`:

```
Net:        100
Gross:        120.0
```

```
$ nantucket test -v
ok    calc_test.prg (0.00s)
      ok    TestAddTax
      ok    TestBadInput
      ok    TestReport
      output:
        | Net:        100
        | Gross:        120.0

PASS  1 file, 3 tests (0.00s)
```

---

## 16. Header files

These headers are built in and can be included without being on disk. A file
of the same name on disk takes precedence.

| Header | Contents |
|---|---|
| `std.ch` | All standard command definitions (always loaded) |
| `set.ch` | `_SET_*` constants for `Set()` (always loaded) |
| `inkey.ch` | Key codes: `K_ENTER`, `K_ESC`, `K_UP`, `K_F1`, `K_CTRL_*`, `K_ALT_*` |
| `hbinkey.ch` | Harbour key code names |
| `box.ch` | Box-drawing strings: `B_SINGLE`, `B_DOUBLE`, ... |
| `common.ch` | `TRUE`, `FALSE`, `YES`, `NO`, `CRLF`, `IS*()` helpers |
| `dbstruct.ch` | `DBS_NAME`, `DBS_TYPE`, `DBS_LEN`, `DBS_DEC` |
| `fileio.ch` | `FO_*` open modes and `FS_*` seek origins |
| `error.ch` | `EG_*` general error codes and `ES_*` severities |
| `achoice.ch` | `AC_*` constants for `AChoice()` user functions |
| `getexit.ch` | `GE_*` constants for GET exit states |
| `setcurs.ch` | `SC_*` cursor shapes |

---

## 17. Environment variables

| Variable | Effect |
|---|---|
| `NANTUCKET_THEME` | Startup theme when `-theme` isn't given |
| `NANTUCKET_PALETTE` | `terminal` uses the terminal's own 16 colours instead of the VGA palette |
| `NANTUCKET_CODEPAGE` | Starting `SET CODEPAGE` (default `AUTO`). See [Code pages](#code-pages) |
| `NANTUCKET_HISTORY` | Dot prompt history file (default `~/.nantucket_history`); `off` keeps no history |
| `NO_COLOR` | When set (and no theme is chosen), use the `mono` theme |
| `NANTUCKET_DEBUG` | When set, internal errors crash with a Go stack trace instead of being reported as runtime errors. For Nantucket developers |

---

## 18. Compatibility notes and limitations

Nantucket aims to run dBase III and Clipper 5 programs unchanged. Known
differences and gaps:

**Not yet supported**

- CDX and IDX indexes, and `INDEX ON ... TAG`
- Classes (`CLASS ... ENDCLASS`) and the `TBrowse`/`TBColumn` objects
- `SET FORMAT`, dBase IV menus and popups
- Designing report and label forms (`CREATE REPORT`, `CREATE LABEL`):
  existing `.FRM` and `.LBL` files can be printed
- Data sources other than DBF, such as SQL or CSV drivers (planned)

**Accepted but with no effect**

- `SET DEBUG`, `SET DOHISTORY`, `SET MENU`, `SET FORMAT`, `SET TALK`
- `SetMode()`, `SetBlink()`, `NoSnow()`, `ColorSelect()`
- `Memory()` and `DiskSpace()` return fixed values; `ErrorLevel()` returns 0
- There is no physical printer; printer output always goes to a file
- The border colour in `SET COLOR`

**Behaviour to be aware of**

- Field names longer than 10 characters match on their first 10 characters.
- File names are matched without regard to case.
- Macros (`&name`) see PRIVATE and PUBLIC variables, not LOCAL or STATIC ones.
- `DO <prog>` recompiles a program automatically if its file has changed.
- The `color` theme is the default on colour terminals. Use `-theme classic`
  for white on black.

---

## 19. Development

### Source layout

```
cmd/nantucket      command line entry point
internal/lex       tokenizer (statements, comments, macros, TEXT)
internal/pp        preprocessor and the built-in std.ch and headers
internal/parse     parser: source to syntax tree
internal/compile   syntax tree to bytecode
internal/vm        bytecode VM, runtime, built-in functions, GET system, UI screens
internal/db        DBF tables, memo files, NTX indexes, work areas
internal/term      consoles (full screen and stream), colours and themes
internal/editor    full-screen text editor (MODIFY COMMAND, MemoEdit)
internal/repl      the dot prompt
internal/testrun   the "nantucket test" runner
examples/          sample programs
```

Programs are preprocessed, parsed and compiled to bytecode, then run by the
VM. Compiled modules are cached and reused until their source file changes.

### Adding a built-in function

Functions are registered in `internal/vm` with `reg("NAME", func(vm *VM, a
[]value.Value) value.Value { ... })` inside an `init()` function. Names are
upper case.

### Adding a command

Most commands are preprocessor rules in `internal/pp/include/std.ch` that
translate to function calls. Add a `#command` rule there, and a function in
`internal/vm` if one is needed.

### Adding a theme

Add an entry to the `Themes` map in `internal/term/theme.go`.

### Tests

```sh
make test                                       # all Go tests
make test-prg                                   # the example .prg tests
go test ./internal/vm -run TestGolden -update   # refresh golden outputs
```

The tests run `.prg` programs in `internal/vm/testdata` and compare their
output with the `.out` files. They also drive the editor, `READ`, `BROWSE`,
`MENU TO`, `CREATE` and the themes through a simulated terminal.

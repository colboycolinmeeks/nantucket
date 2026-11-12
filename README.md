# Nantucket

A dBase III / Clipper compatible environment written in Go, with many
Harbour/xHarbour extensions. It has the classic **dot prompt**, a
full-screen **MODIFY COMMAND** editor, and runs `.prg` programs that use
DBF tables with NTX indexes.

> [!WARNING]
> **Beta software.** This is version 0.2, an early release. Much is still
> to be implemented, and some of what is there may not yet work correctly,
> so please treat it as a beta. Keep backups of any data you use it with.
> See [Not done yet](#not-done-yet) for known gaps, and
> [WHATSNEW.md](WHATSNEW.md) for what has changed. Bug reports and
> suggestions are very welcome.

## Why Nantucket?

I first used dBase at version II, on CP/M machines. Over my career I moved
on to dBase III Plus, then Clipper, and later Harbour, or more specifically
xHarbour. As technology advanced and more powerful systems and software came
to the fore, xBase was all but forgotten.

I still often need to open DBF files. Over the years I've had to hunt for
different ways of doing it, and none of them was as easy as dBase. With the
arrival of AI-assisted development, I decided to write something in Go that
gives a user interface very close to the original dBase.

The bigger picture goes further than DBF files. I want to be able to work
with SQLite and other SQL databases using the xBase commands that many
people will still know well. I've also taken the liberty of extending the
system with some Harbour features. As the project moves forward, I'm also
looking at compiling PRG files into standalone programs.

## About the name

Nantucket isn't just a place on the map. Clipper, the dBase compiler that
so many of us built our careers on, was created by the Nantucket
Corporation, founded in 1984 by Barry ReBell and Brian Russell. Its early
releases were named after the seasons, such as Summer '87, and are still
remembered fondly. Nantucket was later sold to Computer Associates, and
Clipper carried on as CA-Clipper.

Naming this project Nantucket is a nod to the people who created Clipper,
and to everything it made possible for xBase developers.

## Platform support

So far Nantucket has only been built and tested on Linux, specifically
Fedora Linux 44. Windows and macOS are planned, and I'll be testing on both
in due course.

Because it is written in Go, `make dist-all` can already cross-compile
builds for Windows and macOS. These have not been tried on those systems
yet, so expect rough edges, particularly around the full-screen console,
colours and the mouse.

## Build and run

```sh
make build                      # or: go build -o nantucket ./cmd/nantucket

./nantucket                     # interactive dot prompt (full screen)
./nantucket myprog [args...]    # run myprog.prg
./nantucket -c 'USE cust
LIST'                           # run commands and exit
./nantucket -stream myprog      # plain line output (pipes, scripts, CI)
./nantucket -q myprog           # don't wait for a key when the program ends
./nantucket -theme mono         # black and white (also: color, classic)
```

Package a program as a single executable that runs without Nantucket
installed: `./nantucket build myprog` (see
[DOCUMENTATION.md](DOCUMENTATION.md#packaging-a-program)).

Tables can be shared between several users or sessions, with real record
and file locks: `USE customer SHARED`, `RLock()`, `FLock()`, `NetErr()`.

Take the guided tour: `./nantucket examples/demo` walks through creating a
table, editing, browsing, queries, forms, menus and windows. There is also a
small sample app: `./nantucket examples/contacts`.

At the dot prompt, Up/Down recall history, kept between sessions in
`~/.nantucket_history`. Control structures (IF, DO WHILE,
FOR…) can be typed over several lines. `!cmd` runs a shell command. Ctrl-C
(or Alt-C) interrupts a running program.

## Writing programs

```
. MODIFY COMMAND hello
```

opens the editor on `hello.prg`. Editor keys:

| Key | Action |
| --- | --- |
| Ctrl-W, Ctrl-End, F10 | save and exit |
| Esc, Ctrl-Q | exit (asks before discarding changes) |
| F2 | save and keep editing |
| F7 / Ctrl-L | find / find again |
| F5 | go to line |
| Ctrl-Y / Ctrl-N / Ctrl-T | delete line / insert line / delete word |
| Ctrl-U | undo |
| Shift+arrows, Ctrl-Ins / Shift-Del / Shift-Ins | select, copy / cut / paste |
| Ctrl-K B, K, then C / V / Y / H | mark a block, then copy / move / delete / hide it |
| Ctrl-Left/Right, Ctrl-PgUp/PgDn | word left/right, top/bottom |
| F1 | help |

The editor highlights xBase syntax. When you save a `.prg` it is compiled,
and if there's an error the editor stays open on that line. `DO hello` runs it and
automatically recompiles a program whose file has changed.

## What works

**Language** (Clipper 5 / Harbour level)
- `FUNCTION`/`PROCEDURE` (incl. `STATIC`), `PARAMETERS`, `LOCAL`, `STATIC`,
  `PRIVATE`, `PUBLIC`, `MEMVAR`, `FIELD`, `DECLARE`/`DIMENSION`
- `IF/ELSEIF/ELSE`, `DO WHILE`, `FOR…NEXT [STEP]`, `DO CASE`, `EXIT`, `LOOP`,
  `BEGIN SEQUENCE / RECOVER USING / BREAK`, `DO proc WITH` (by reference)
- xHarbour: `FOR EACH`, `SWITCH`, `TRY/CATCH/FINALLY`, hashes `{"k" => v}`,
  `+= -= *= /= ++ --`, `0dYYYYMMDD` date literals
- Code blocks with detached locals, arrays, `@var` by-reference arguments
- Macros: `&var`, `&var.ext`, `&(expr)`, statement macros (`&cCmd`),
  `TEXT…ENDTEXT` with substitution
- dBase 4-letter abbreviations (`DISP STRU`, `MODI COMM`, `REPL`, `ENDD`…)
- Full Clipper preprocessor: `#define`, `#include`, `#ifdef`, `#command`,
  `#xcommand`, `#translate`, `#xtranslate`. All the dBase commands are defined
  as rules in the built-in `std.ch`, so users can add their own commands
- Built-in headers: `inkey.ch`, `set.ch`, `box.ch`, `common.ch`,
  `dbstruct.ch`, `fileio.ch`, `error.ch`, `achoice.ch`, `getexit.ch`
- Runtime errors as Clipper Error objects, `ErrorBlock()`, call traces
- Full screen debugger (Alt-D, `AltD()`, `nantucket -debug`): stepping,
  breakpoints, watches, the call stack and every variable, LOCALs included

**Data**
- DBF (dBase III/IV, Clipper, FoxPro read), memo files DBT/FPT
- NTX indexes (Clipper format, read/write), `UNIQUE`, `FOR`, multiple
  indexes per table, numeric/date/character keys
- `USE`, `SELECT`, aliases (`cust->name`, `cust->(expr)`), 250 work areas
- `APPEND [BLANK|FROM]`, `REPLACE`, `DELETE`, `RECALL`, `PACK`, `ZAP`,
  `GO`, `SKIP`, `SEEK`, `FIND`, `LOCATE`/`CONTINUE`, scopes
  (`ALL/NEXT/RECORD/REST/FOR/WHILE`)
- `LIST`, `DISPLAY`, `COUNT`, `SUM`, `AVERAGE`, `TOTAL`, `SORT`, `COPY TO`
  (DBF/SDF/DELIMITED), `COPY STRUCTURE [EXTENDED]`, `CREATE FROM`
- `SET FILTER`, `SET RELATION`, `SET DELETED`, `SET ORDER`, `REINDEX`
- Interactive `CREATE` / `MODIFY STRUCTURE`, `BROWSE`, `EDIT`, `APPEND`

**Screen and keyboard**
- `?`, `??`, `@…SAY…GET`, `PICTURE` templates and functions, `VALID`,
  `WHEN`, `RANGE`, `READ`, `@…TO`, `@…BOX`, `@…CLEAR`, `@…PROMPT`/`MENU TO`
- `ACCEPT`, `INPUT`, `WAIT`, `Inkey()`, `KEYBOARD`, `SET KEY`, `SET COLOR`,
  `SaveScreen()`/`RestScreen()`, `Alert()`, `AChoice()`, `MemoEdit()`
- `SET ALTERNATE`, `SET PRINTER` (to a file), `SET CONSOLE`

**Library**: about 300 functions covering strings, numbers, dates, arrays,
hashes, files (`FOpen`/`FRead`/…), memory files (`SAVE`/`RESTORE`), and more.

## Themes

Three colour themes are built in:

| Theme     | Look |
|-----------|------|
| `color`   | white on blue, GETs in white on green (default on colour terminals) |
| `classic` | white on black, the plain Clipper default |
| `mono`    | black and white only, for monochrome terminals |

The theme is picked from `-theme <name>`, then `$NANTUCKET_THEME`, and
otherwise from the terminal: `mono` when `NO_COLOR` is set or the terminal
has fewer than 8 colours, else `color`.

A theme sets the default `SET COLOR` string, so programs can still override it:

```
SET COLOR TO GR+/B,W+/R     && override (slots left out keep the theme's colours)
SET COLOR TO                && back to the theme's colours
SET THEME TO mono           && switch theme (resets colours, clears the screen)
SET THEME TO                && back to the startup theme
? SetTheme(), IsColor()     && current theme name; .F. in mono
```

In `mono` every colour is shown as normal, bright or reverse video, so
programs written for colour stay readable. The editor, BROWSE headers and
ALERT() boxes take their colours from the theme.

Colours are drawn with the classic VGA palette (blue is `#0000AA`, white is
`#FFFFFF`, and so on), so they look the same whatever colour theme the
terminal uses. To use the terminal's own 16 colours instead, set
`NANTUCKET_PALETTE=terminal`. The `mono` theme always uses the terminal's
colours.

## Windows

`HELP` opens the command summary in a window that stays on screen while you
work: drag its title bar to move it, drag its corner to resize it, use the
`[_][↑][x]` controls to minimise, maximise or close it, and scroll it with
the mouse wheel. Programs can open their own numbered windows; while one is
selected, every screen command works inside it:

```
WINDOW OPEN 2, 5 TO 12, 45 TITLE "Customers" TO nCust
? "Inside the window"
SET WINDOW TO               && back to the full screen (window 0)
WINDOW CLOSE nCust
```

## Status bar

At the dot prompt the bottom line is a status bar showing the current work
area, the table in use, the record number and total records, the
controlling index, whether a filter is set, the current directory, and
the date and time. `SET STATUS OFF` hides it.

Type `VERSION` for the version number, or `VERSION FULL` to open a window
showing what's new in this release (see [WHATSNEW.md](WHATSNEW.md)).

## Testing your programs

`nantucket test` runs every `*_test.prg` file. Each `PROCEDURE Test...` is a
test, `ASSERT` and `AssertEquals()` check values, and a `<name>_test.out`
file, if present, must match the program's output:

```
PROCEDURE TestTotal
   AssertEquals(17, 12 + 5)
   ASSERT Upper("a") == "A" MESSAGE "Upper() works"
RETURN
```

```sh
./nantucket test              # all tests under the current directory
./nantucket test -update      # save current output as the expected .out files
```

See `examples/tests` and the "Testing programs" section of DOCUMENTATION.md.
COMMANDS.md lists every command.

## Make targets

```sh
make            # fmt check, vet, test, build, .prg tests (same as `make all`)
make build      # build ./nantucket
make test       # run all tests
make test-prg   # run the example .prg tests (nantucket test examples)
make test-race  # tests with the race detector
make cover      # tests with a coverage report (coverage.html)
make vet        # go vet
make fmt        # gofmt the source
make run        # build and start the dot prompt
make demo       # build and run the guided tour (examples/demo)
make example    # build and run examples/contacts
make dist       # release archive (binary, LICENSE, NOTICE, docs, examples)
make dist-all   # release archives for Linux, macOS and Windows
make install    # go install into $GOBIN
make clean      # remove build outputs
make help       # list targets
```

## Layout

```
cmd/nantucket      command line entry point
internal/lex       tokenizer (statements, xBase comments, macros, TEXT)
internal/pp        preprocessor + embedded std.ch and headers
internal/parse     parser -> AST
internal/compile   AST -> bytecode
internal/vm        bytecode VM and runtime core: language, built-in library,
                   work areas, locks, PICTURE formatting, extension API
internal/gets      GET system: @ ... GET, READ, MENU TO, Alert(), AChoice()
internal/screens   BROWSE, EDIT, CREATE, MODIFY, report and label designers,
                   windows, HELP, status bar
internal/dbcmd     LIST, COUNT/SUM, COPY TO, APPEND FROM, SORT, TOTAL...
internal/debugger  the debugger: Alt-D, AltD(), nantucket -debug
internal/forms     REPORT FORM, LABEL FORM, CREATE REPORT/LABEL ... FROM
internal/report    reading and writing .FRM / .LBL files
internal/features  links the packages above into the runtime (and their tests)
internal/vmtest    helpers for testing xBase code from Go
internal/db        DBF tables, memo files, NTX indexes, work areas, locking
internal/term      console: full screen (tcell) and stream implementations
internal/editor    full screen text editor (MODI COMM, MemoEdit)
internal/repl      the dot prompt
internal/testrun   the "nantucket test" runner
internal/app       "nantucket build": packaging programs as executables
examples/          sample programs
```

The core (`internal/vm`) never imports the feature packages: each of them
registers its commands with `vm.Register` when it is initialised, and
`internal/features` links them all in. New commands can live in their own
package the same way.

Programs are compiled to bytecode (`compile.Module`). `nantucket build`
(`internal/app`) packages a program's sources into a copy of the
executable; a later phase may package the compiled bytecode instead.

## Not done yet

- CDX/IDX indexes, `INDEX ON … TAG`
- Clipper/Harbour classes (`CLASS … ENDCLASS`), TBrowse/TBColumn objects
- `SET FORMAT`, dBase IV menus/popups
- Data sources other than DBF (planned: SQL, CSV via a driver interface)

## Tests

```sh
go test ./...
go test ./internal/vm -run TestGolden -update   # refresh golden outputs
```

The tests run `.prg` programs against golden output and drive the
full-screen editor, READ, BROWSE, MENU TO and CREATE through a simulated
terminal.

## Licence

Nantucket is copyright 2026 Colin Meeks and is licensed under the
[Apache License, Version 2.0](LICENSE).

You are free to use, change and distribute it, including in commercial
work. If you distribute Nantucket or a work derived from it, keep the
[LICENSE](LICENSE) and [NOTICE](NOTICE) files (or the NOTICE attribution
text) with it, and mark any files you have changed.

Nantucket is provided "as is", without warranty of any kind, as set out in
sections 7 and 8 of the [Apache License](LICENSE). It reads and writes your
data files directly, so keep backups of any data you care about,
particularly when trying it on existing dBase or Clipper tables.

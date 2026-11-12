# Nantucket Command Reference

This is the complete list of commands and statements in Nantucket, with the
syntax and an example for each. For the language, functions, colours and
databases in depth, see [DOCUMENTATION.md](DOCUMENTATION.md).

## Conventions

| Notation | Meaning |
|---|---|
| `UPPER CASE` | Keywords. Type them in any case |
| `<name>` | Something you supply: a file name, expression, variable... |
| `[ ... ]` | Optional |
| `a \| b` | One of the alternatives |
| `...` | The previous item can repeat |
| `<scope>` | Any of the [scope clauses](#scope-clauses) |

- Keywords can be shortened to their **first four letters**: `DISP STRU`,
  `MODI COMM`, `REPL`, `ENDD`, `RETU`.
- File names can be given without an extension; the usual one (`.dbf`,
  `.prg`, `.ntx`, `.mem`...) is added.
- File names can come from a variable with a macro (`USE &cFile`) or in
  parentheses (`USE (cFile)`).
- Put several commands on one line with `;` between them. End a line with
  `;` to continue a long command on the next line.

### Scope clauses

Commands that work on a range of records accept these clauses, in any order:

| Clause | Records processed |
|---|---|
| `ALL` | Every record (the default for most commands) |
| `NEXT <n>` | The next `n` records, starting with the current one |
| `RECORD <n>` | Only record `n` |
| `REST` | From the current record to the end |
| `FOR <condition>` | Only records where the condition is true |
| `WHILE <condition>` | Starting at the current record, while the condition stays true |

```
LIST name, city FOR state = "MA"
REPLACE ALL price WITH price * 1.1 FOR category = "B"
DELETE NEXT 5
COUNT TO n WHILE custno = "1001"
```

---

## Contents

- [Program structure](#program-structure)
- [Variables](#variables)
- [Control structures](#control-structures)
- [Output](#output)
- [Screen](#screen)
- [Windows](#windows)
- [Keyboard and input](#keyboard-and-input)
- [Running programs](#running-programs)
- [Memory variables](#memory-variables)
- [Opening and closing tables](#opening-and-closing-tables)
- [Table structure](#table-structure)
- [Moving through records](#moving-through-records)
- [Adding and changing records](#adding-and-changing-records)
- [Listing and totals](#listing-and-totals)
- [Indexes](#indexes)
- [Filters and relations](#filters-and-relations)
- [Copying, importing and sorting](#copying-importing-and-sorting)
- [Files and directories](#files-and-directories)
- [Testing](#testing)
- [SET commands](#set-commands)
- [Dot prompt only](#dot-prompt-only)
- [Preprocessor directives](#preprocessor-directives)
- [Alphabetical index](#alphabetical-index)

---

## Program structure

### FUNCTION, PROCEDURE

```
[STATIC] FUNCTION <name>( [<param>, ...] )
   <statements>
RETURN <value>

[STATIC] PROCEDURE <name>( [<param>, ...] )
   <statements>
RETURN
```

Defines a function (returns a value) or a procedure (doesn't). `STATIC`
makes it visible only inside its own `.prg` file. Statements at the top of a
file, before the first `FUNCTION` or `PROCEDURE`, form the file's main
procedure.

```
FUNCTION Area(nWidth, nHeight)
RETURN nWidth * nHeight
```

`INIT PROCEDURE` runs before the main program starts (procedures in a file
loaded later with `SET PROCEDURE TO` run when it is loaded). `EXIT
PROCEDURE` runs when the program ends, including after `QUIT`. Neither
takes parameters.

### RETURN

```
RETURN [<value>]
```

Leaves the current function or procedure, optionally returning a value.

### RETURN TO MASTER

Stops the program and returns to the dot prompt. Same as `CANCEL`.

### NOTE, comments

```
* comment             (at the start of a line)
NOTE comment          (at the start of a line)
<command>  && comment
<command>  // comment
/* block
   comment */
```

### REQUEST, ANNOUNCE, EXTERNAL

```
REQUEST <name>, ...
```

Accepted for Clipper compatibility and ignored. Nantucket finds functions
when they are called.

---

## Variables

### LOCAL

```
LOCAL <name> [:= <value>] [, ...]
```

Variables visible only in the current function. Must come before other
statements in the function.

```
LOCAL nTotal := 0, aItems := {}
```

### STATIC

```
STATIC <name> [:= <value>] [, ...]
```

Like `LOCAL`, but the value is kept between calls. At the top of a file
(outside any function), the variable is shared by every function in the file.

### PRIVATE

```
PRIVATE <name> [:= <value>] [, ...]
PRIVATE &<macro>
```

Visible in the current routine and every routine it calls. Released when
the routine returns.

### PUBLIC

```
PUBLIC <name> [:= <value>] [, ...]
```

Visible everywhere until released. A new public variable starts as `.F.`.

### PARAMETERS

```
PARAMETERS <name>, ...
```

dBase-style parameters, created as private variables. `DO ... WITH` passes
variables by reference, so changing a parameter changes the caller's variable.

```
PROCEDURE AddTax
PARAMETERS nAmount
nAmount := nAmount * 1.05
RETURN
```

### MEMVAR, FIELD

```
MEMVAR <name>, ...
FIELD <name>, ... [IN <alias>]
```

Declare that names refer to memory variables or to database fields.

### DECLARE, DIMENSION

```
DECLARE <name>[<n>[, <m>...]] [, ...]
DIMENSION <name>[<n>[, <m>...]] [, ...]
```

Creates private arrays filled with `NIL`.

```
DECLARE aMonths[12], aGrid[10, 10]
```

### STORE

```
STORE <value> TO <var> [, <var>...]
```

Assigns one value to several variables.

```
STORE 0 TO nCount, nTotal
```

### DEFAULT

```
DEFAULT <var> TO <value> [, <var> TO <value>...]
```

Assigns a value only if the variable is `NIL`. Useful for optional
parameters.

```
FUNCTION Greet(cName)
   DEFAULT cName TO "world"
RETURN "Hello, " + cName
```

---

## Control structures

### IF

```
IF <condition>
   <statements>
[ELSEIF <condition>
   <statements>]
[ELSE
   <statements>]
ENDIF | END | END IF
```

### DO CASE

```
DO CASE
CASE <condition>
   <statements>
[CASE <condition>
   <statements>]
[OTHERWISE
   <statements>]
ENDCASE
```

Runs the first `CASE` whose condition is true.

### DO WHILE

```
DO WHILE <condition> | WHILE <condition>
   <statements>
   [LOOP]
   [EXIT]
ENDDO | END | ENDWHILE
```

`LOOP` jumps back to the condition; `EXIT` leaves the loop.

```
GO TOP
DO WHILE !EOF()
   ? name
   SKIP
ENDDO
```

### FOR ... NEXT

```
FOR <var> := <start> TO <end> [STEP <n>]
   <statements>
NEXT | ENDFOR
```

`=` can be used instead of `:=`. A negative `STEP` counts down.

### FOR EACH

```
FOR EACH <var> IN <array | hash | string>
   <statements>
NEXT
```

Goes through each element of an array, each value of a hash, or each
character of a string.

### SWITCH

```
SWITCH <expression>
CASE <value>
   <statements>
   [EXIT]
[CASE <value> ...]
[OTHERWISE
   <statements>]
ENDSWITCH | END
```

Without `EXIT`, execution falls through to the next `CASE`.

### EXIT, LOOP

Leave a loop (`EXIT`) or start its next iteration (`LOOP`). `EXIT` also
ends a `SWITCH` case.

### BEGIN SEQUENCE

```
BEGIN SEQUENCE [WITH <error block>]
   <statements>
   [BREAK [<value>]]
[RECOVER [USING <var>]
   <statements>]
END [SEQUENCE]
```

`BREAK` jumps to `RECOVER`, storing its value in the `USING` variable.
With `WITH {|e| Break(e)}`, any runtime error also jumps to `RECOVER`, with
the Error object.

```
BEGIN SEQUENCE WITH {|e| Break(e)}
   USE missing
RECOVER USING oErr
   ? "Could not open:", oErr:description
END SEQUENCE
```

### TRY ... CATCH

```
TRY
   <statements>
[CATCH [<var>]
   <statements>]
[FINALLY
   <statements>]
END | ENDTRY
```

Catches runtime errors. `FINALLY` always runs.

### BREAK

```
BREAK [<value>]
```

Jumps to the `RECOVER` of the enclosing `BEGIN SEQUENCE`.

---

## Output

### ? and ??

```
? [<expression>, ...]
?? [<expression>, ...]
```

`?` starts a new line and prints the values separated by spaces. `??`
prints without a new line. Output also goes to the alternate and printer
files when those are on.

```
? "Total:", nTotal
?? " (approx.)"
```

### TEXT ... ENDTEXT

```
TEXT
<lines of text>
ENDTEXT
```

Prints the lines as they are, expanding `&macros`.

```
TEXT TO FILE <file>
TEXT TO PRINTER
```

also write the lines to a new file (`.txt` by default) or the printer.

### EJECT

Sends a form feed to the printer file and resets `PRow()`/`PCol()` to 0.

---

## Screen

### CLEAR, CLS

```
CLEAR
CLEAR SCREEN
CLS
```

`CLS` and `CLEAR SCREEN` clear the screen and move the cursor to 0, 0.
`CLEAR` also discards pending GETs.

### @ ... SAY

```
@ <row>, <col> SAY <expression> [PICTURE <picture>] [COLOR <color>]
```

Displays a value at a position. With `SET DEVICE TO PRINTER` it goes to the
printer file instead.

```
@ 2, 10 SAY "Customer:" COLOR "W+/B"
@ 3, 10 SAY nBalance PICTURE "@E 999,999.99"
```

### @ ... GET

```
@ <row>, <col> GET <var> [PICTURE <picture>] [VALID <condition>]
     [WHEN <condition>] [RANGE <low>, <high>] [COLOR <color>]
```

Creates an entry field for a variable or field. Nothing is edited until
`READ`.

### @ ... SAY ... GET

```
@ <row>, <col> SAY <prompt> [PICTURE <p>] [COLOR <c>] GET <var> [<GET clauses>]
```

Shows a prompt with the GET one column after it.

```
@ 5, 5 SAY "Age" GET nAge PICTURE "999" RANGE 0, 120
READ
```

### READ

```
READ [SAVE]
```

Lets the user edit all pending GETs. `SAVE` keeps the GETs for another
`READ`. See [DOCUMENTATION.md, GETs and READ](DOCUMENTATION.md#gets-and-read)
for the keys.

### CLEAR GETS

Discards pending GETs without editing them.

### @ ... CLEAR

```
@ <row>, <col>
@ <row>, <col> CLEAR
@ <top>, <left> CLEAR TO <bottom>, <right>
```

Clears the rest of a row, the area from a point to the bottom right, or a
rectangle.

### @ ... TO

```
@ <top>, <left> TO <bottom>, <right> [DOUBLE] [COLOR <color>]
```

Draws a single- or double-line box.

### @ ... BOX

```
@ <top>, <left>, <bottom>, <right> BOX <characters> [COLOR <color>]
```

Draws a box from up to nine characters: the corners and sides clockwise from
the top left, then an optional fill character. `box.ch` has ready-made
strings.

### @ ... PROMPT and MENU TO

```
@ <row>, <col> PROMPT <text> [MESSAGE <message>]
MENU TO <var>
```

`@ ... PROMPT` defines menu items and `MENU TO` lets the user choose one.
The variable gets the item number, or 0 if Esc is pressed. Its value before
`MENU TO` selects the starting item.

```
nChoice := 1
@ 5, 10 PROMPT " Add    " MESSAGE "Add a new record"
@ 6, 10 PROMPT " Browse " MESSAGE "Browse the table"
@ 7, 10 PROMPT " Quit   "
MENU TO nChoice
```

---

## Windows

Numbered windows sit on top of the full screen (window 0). While one is
selected, all screen commands work inside it, relative to its inside. See
[DOCUMENTATION.md, Windows](DOCUMENTATION.md#windows) for the details,
mouse use and the checks made. At the dot prompt, F6 arranges windows with
the keyboard and F1 opens or closes `HELP`.

### WINDOW OPEN

```
WINDOW OPEN <top>, <left> TO <bottom>, <right> [TITLE <title>] [COLOR <color>] [FIXED] [TO <var>]
```

Opens a framed window and selects it. `FIXED` stops the user resizing or
maximising it. `TO` stores its number. Same as `WOpen()`.

### SET WINDOW TO

```
SET WINDOW TO [<n>]
```

Selects window `n` and brings it to the front. No number (or 0): the full
screen. Same as `WSelect()`.

### WINDOW CLOSE

```
WINDOW CLOSE [<n>]
WINDOW CLOSE ALL
```

Closes a window (default: the current one), or all of them.

### WINDOW MOVE, WINDOW SIZE

```
WINDOW MOVE <n> TO <top>, <left>
WINDOW SIZE <n> TO <rows>, <cols>
```

Moves a window's frame, or sets the frame size.

### WINDOW MINIMIZE, MAXIMIZE, RESTORE

```
WINDOW MINIMIZE [<n>]
WINDOW MAXIMIZE [<n>]
WINDOW RESTORE [<n>]
```

Default: the current window. Only sizable windows can be maximised.

## Keyboard and input

### WAIT

```
WAIT [<prompt>] [TO <var>]
```

Shows a prompt (default "Press any key to continue...") and waits for one
key. `TO` stores the key as a one-character string.

### ACCEPT

```
ACCEPT [<prompt>] TO <var>
```

Reads a line of text into a character variable.

### INPUT

```
INPUT [<prompt>] TO <var>
```

Reads an expression, evaluates it and stores the result, so `42` gives a
number and `"abc"` a string.

### KEYBOARD

```
KEYBOARD <string | number | array>
```

Clears the keyboard buffer and fills it with keys. A `;` in a string stands
for Enter. At most `SET TYPEAHEAD` keys are stored.

```
KEYBOARD "Smith;"
```

### CLEAR TYPEAHEAD

Empties the keyboard buffer.

### SET KEY

```
SET KEY <key code> TO [<procedure>]
```

Calls a procedure whenever a key is pressed while waiting for input. The
procedure receives the calling procedure's name, line and the current GET
variable name. Without a procedure, the key is released.

```
#include "inkey.ch"
SET KEY K_F1 TO ShowHelp
```

### SET FUNCTION

```
SET FUNCTION <n> [TO] <string>
```

Makes function key `n` (1–10) type the string. A `;` stands for Enter.

```
SET FUNCTION 2 TO "LIST;"
```

---

## Running programs

### DO

```
DO <program | procedure> [WITH <argument>, ...]
```

Runs a procedure, or the program file `<name>.prg`. Arguments that are
variables are passed by reference. A program whose file has changed is
recompiled first.

```
DO report WITH "MA", dFrom
```

### SET PROCEDURE

```
SET PROCEDURE TO [<file>] [ADDITIVE]
CLOSE PROCEDURE
```

Loads a file of procedures and functions so other programs can call them.
Without `ADDITIVE`, earlier procedure files are closed first.

### COMPILE

```
COMPILE <file>
```

Compiles a program without running it and reports any errors.

### CANCEL

Stops the program and returns to the dot prompt (or exits when running a
program from the command line).

### QUIT

Closes all files and leaves Nantucket.

### RUN, !

```
RUN <command>
! <command>
```

Runs an operating-system command. `!` on its own at the dot prompt opens a
shell.

```
RUN ls -l *.dbf
```

---

## Memory variables

### RELEASE

```
RELEASE <var>, ...
RELEASE ALL [LIKE <skeleton> | EXCEPT <skeleton>]
```

Releases variables. `ALL` releases the private variables of the current
routine. Skeletons use `*` and `?` as wildcards.

```
RELEASE ALL LIKE temp*
```

### SAVE

```
SAVE TO <file> [ALL [LIKE <skeleton> | EXCEPT <skeleton>]]
SAVE ALL LIKE <skeleton> TO <file>
```

Saves private and public variables to a `.mem` file.

### RESTORE

```
RESTORE FROM <file> [ADDITIVE]
```

Loads variables from a `.mem` file. Without `ADDITIVE`, existing private and
public variables are released first.

### DISPLAY MEMORY, LIST MEMORY

Lists every memory variable with its scope, type and value.

### CLEAR MEMORY

Releases all private and public variables.

### CLEAR ALL

Closes all tables and the alternate file, releases all memory variables and
discards pending GETs.

---

## Opening and closing tables

### USE

```
USE [<file>] [ALIAS <alias>] [NEW] [EXCLUSIVE | SHARED] [READONLY]
    [INDEX <index>, ...] [VIA <driver>]
```

Opens a table in the current work area, closing any table already open
there. `NEW` uses the next free work area instead. `USE` with no file closes
the current table.

```
USE customer INDEX custname, custno
USE orders ALIAS ord NEW
```

`EXCLUSIVE` opens the table for this session alone (the default, while
`SET EXCLUSIVE` is ON). `SHARED` lets other users open it too; changing a
record then needs a lock (`RLock()` or `FLock()`). If the open conflicts
with another user, the work area is left empty and `NetErr()` returns
`.T.` instead of an error being raised. `READONLY` opens the table for
reading only. `VIA` is accepted and ignored. See
[DOCUMENTATION.md, Sharing tables](DOCUMENTATION.md#sharing-tables-between-users).

### SELECT

```
SELECT <work area number | alias>
SELECT 0
```

Makes another work area current. `SELECT 0` chooses the lowest unused area.

### CLOSE

```
CLOSE [<alias>]
CLOSE DATABASES
CLOSE INDEXES
CLOSE ALTERNATE
CLOSE PROCEDURE
CLOSE FORMAT
CLOSE ALL
```

| Form | Closes |
|---|---|
| `CLOSE` | The table in the current work area |
| `CLOSE <alias>` | The table with that alias |
| `CLOSE DATABASES` | All tables and their indexes |
| `CLOSE INDEXES` | The indexes of the current table |
| `CLOSE ALTERNATE` | The alternate file |
| `CLOSE PROCEDURE` | Procedure files |
| `CLOSE FORMAT` | Accepted and ignored |
| `CLOSE ALL` | All tables, the alternate file and the printer file |

### COMMIT

Writes all changed records to disk.

### UNLOCK

```
UNLOCK [ALL]
```

Writes the current record and releases the record and file locks of the
current work area (`ALL`: every work area). Locks only matter on tables
opened `SHARED`.

---

## Table structure

### CREATE

```
CREATE <file>
CREATE <file> FROM <structure file> [NEW] [ALIAS <alias>]
```

`CREATE <file>` opens the interactive structure editor; when you save, it
offers to add records. `CREATE ... FROM` builds a table from a
structure-extended table (see `COPY STRUCTURE EXTENDED`).

Structure editor keys: Enter edits a field, Ctrl-N inserts, Ctrl-U deletes,
Ctrl-W saves, Esc cancels. Field types: `C` character, `N` numeric, `D` date,
`L` logical, `M` memo.

### MODIFY STRUCTURE

Opens the structure editor on the current table. When you save, the table
is rebuilt and existing data is copied into fields with the same names.

### DISPLAY STRUCTURE, LIST STRUCTURE

```
DISPLAY STRUCTURE [TO PRINTER]
LIST STRUCTURE [TO PRINTER]
```

Shows the file name, record count, last update, open indexes, and each
field's name, type, width and decimals.

### COPY STRUCTURE

```
COPY STRUCTURE TO <file> [FIELDS <field>, ...]
COPY STRUCTURE EXTENDED TO <file>
```

`COPY STRUCTURE` creates an empty table with the same fields (or just the
fields listed). `EXTENDED` creates a table with one record per field, in the
fields `FIELD_NAME`, `FIELD_TYPE`, `FIELD_LEN` and `FIELD_DEC`.

### DISPLAY STATUS, LIST STATUS

Shows the open tables, their aliases, indexes, filters and relations, and
the main settings including the theme and colours.

---

## Moving through records

### GO, GOTO

```
GO[TO] <record number>
GO[TO] TOP
GO[TO] BOTTOM
```

Moves to a record by number, or to the first or last record in index order.

### SKIP

```
SKIP [<n>] [ALIAS <alias>]
```

Moves forward `n` records (default 1), or backward if `n` is negative.

### SEEK

```
SEEK <expression> [SOFTSEEK] [LAST]
```

Searches the controlling index for a key. `Found()` tells you whether it
matched. `SOFTSEEK` (or `SET SOFTSEEK ON`) stops at the next higher key when
there is no match; `LAST` finds the last record with the key.

```
SEEK Upper(cName)
IF !Found()
   ? "Not found"
ENDIF
```

### FIND

```
FIND <text>
```

Like `SEEK`, but the key is literal text and needs no quotes. Use
`FIND &cVar` to search for a variable's value.

### LOCATE

```
LOCATE [FOR <condition>] [WHILE <condition>] [<scope>]
```

Searches forward for the first record matching the condition, without an
index. `Found()` reports the result.

### CONTINUE

Finds the next record matching the last `LOCATE`.

```
LOCATE FOR city = "Boston"
DO WHILE Found()
   ? name
   CONTINUE
ENDDO
```

---

## Adding and changing records

### APPEND BLANK

Adds an empty record and makes it current.

### APPEND

Adds records with a full-screen form, one record per page. Esc stops.

### APPEND FROM

```
APPEND FROM <file> [FIELDS <field>, ...] [<scope>]
    [SDF | DELIMITED [WITH <delimiter> | WITH BLANK | WITH TAB | WITH PIPE]]
```

Adds records from another table or a text file. Fields are matched by name.
`SDF` reads fixed-width lines, and `DELIMITED` reads comma-separated values
with quoted strings.

```
APPEND FROM export.txt DELIMITED
APPEND FROM oldcust FOR state = "MA"
```

### REPLACE

```
REPLACE <field> WITH <value> [, <field> WITH <value>...] [<scope>]
```

Changes fields in the current record, or in every record in the scope.

```
REPLACE price WITH price * 1.1, updated WITH Date() FOR category = "B"
```

### DELETE

```
DELETE [<scope>]
```

Marks records for deletion. They stay in the table until `PACK`. With `SET
DELETED ON` they are hidden.

### RECALL

```
RECALL [<scope>]
```

Removes the deletion mark.

### PACK

Removes records marked for deletion and rebuilds the open indexes.

### ZAP

Removes every record. With `SET SAFETY ON`, asks first in interactive use.

### EDIT, CHANGE

Edits records with a full-screen form. PgUp/PgDn move between records,
Ctrl-U marks or unmarks for deletion, Ctrl-N adds a record, Ctrl-W saves
and Esc exits.

### BROWSE

```
BROWSE [FIELDS <field>, ...]
```

Shows records in a scrolling table for viewing and editing. Arrows and Tab
move, Enter edits a cell, Ctrl-N adds a record, Ctrl-U marks or unmarks for
deletion, Esc or Ctrl-W exits. Memo fields open in the editor.

---

## Listing and totals

### REPORT FORM

```
REPORT FORM <file> [HEADING <text>] [PLAIN] [SUMMARY] [NOEJECT]
    [TO PRINTER] [TO FILE <file>] [NOCONSOLE]
    [<scope>] [FOR <condition>] [WHILE <condition>]
```

Prints the current table using a dBase III / Clipper report form
(`.frm`): page number and date, the report's headings, column headings,
one line per record (long text wraps within its column), group and
subgroup headings with subtotals, and a grand total for columns marked
for totals. `HEADING` adds a heading line, `PLAIN` leaves out the page
number, date and page breaks, `SUMMARY` prints only the group lines and
totals, and `NOEJECT` skips the form feed before the report. Sent to a
printer or file, the report is split into pages of the form's length.

### LABEL FORM

```
LABEL FORM <file> [SAMPLE] [TO PRINTER] [TO FILE <file>] [NOCONSOLE]
    [<scope>] [FOR <condition>] [WHILE <condition>]
```

Prints labels using a dBase III / Clipper label form (`.lbl`). Each label
line lists expressions separated by commas, printed joined by single
spaces; lines that come out empty are dropped. `SAMPLE` first prints a row
of labels filled with `*` for lining up the paper.

### CREATE REPORT, MODIFY REPORT

```
CREATE REPORT [<file>]
MODIFY REPORT [<file>]
CREATE REPORT <file> FROM <field>, ...
```

Opens the report designer on a report form (`.frm`), new or existing; with
no file name it asks for one. The designer has three pages:

- **F2 Layout**: up to four page heading lines (printed centred), page
  width, lines per page, margins, double spacing, summary only, plain (no
  page numbers or date) and page breaks before and after the report.
- **F3 Groups**: the expression to group records on and the heading
  printed before each group, a page break after each group, and a
  subgroup with its own heading. The table should be indexed or sorted on
  the group expression.
- **F4 Columns**: one line per column with its contents (any expression),
  heading (`;` starts a new heading line), width, decimals and whether to
  total it. Naming a field of the current table fills in its width,
  decimals and heading, and numeric fields are totalled. Ctrl-N inserts a
  column and Ctrl-U deletes one; a report has up to 24 columns.

Enter edits the highlighted setting (or just start typing); Space switches
a Yes/No setting. The line above the keys shows the report's width
against the page's. Ctrl-W saves, after checking every expression; Esc
leaves, asking first if anything changed. Expressions that would run code
(such as calling `FErase()`) are refused unless `SET TRUSTED` is ON, as
`REPORT FORM` would refuse them.

`CREATE REPORT <file> FROM <fields>` makes a simple report without the
designer: one column for each field of the current table, with the
field's name as its heading and numeric fields totalled. `SET SAFETY ON`
asks before replacing an existing file.

### CREATE LABEL, MODIFY LABEL

```
CREATE LABEL [<file>]
MODIFY LABEL [<file>]
CREATE LABEL <file> FROM <field>, ...
```

Opens the label designer on a label form (`.lbl`), new or existing: a
remark, the label's height (1 to 16 lines) and width, the left margin,
blank lines and spaces between labels, how many labels across, and the
contents of each line. A line lists expressions separated by commas, such
as `Trim(first), last`; `LABEL FORM` prints them joined by single spaces.
New labels start at dBase's standard size, 35 characters by 5 lines, one
across. Keys and checks are as for `CREATE REPORT`.

`CREATE LABEL <file> FROM <fields>` makes a label with one field of the
current table on each line (character fields trimmed).


### LIST, DISPLAY

```
LIST [<expression>, ...] [OFF] [<scope>] [TO PRINTER | TO FILE <file>]
DISPLAY [<expression>, ...] [OFF] [<scope>] [TO PRINTER | TO FILE <file>]
```

Prints records. `LIST` covers all records by default. `DISPLAY` shows the
current record only unless a scope is given, and pauses after each screenful.
`OFF` hides the record number. With no expressions, all fields are shown;
memo fields appear as `Memo` (or `memo` when empty) unless named
explicitly, in which case they are wrapped at `SET MEMOWIDTH`.

```
LIST name, city, Upper(state) FOR balance > 1000 OFF
DISPLAY ALL TO FILE report.txt
```

### COUNT

```
COUNT [TO <var>] [<scope>]
```

Counts records in the scope. Without `TO`, prints the count.

### SUM

```
SUM [<expression>, ...] [TO <var>, ...] [<scope>]
```

Adds up numeric expressions over the scope.

```
SUM qty, qty * price TO nQty, nValue FOR region = "East"
```

### AVERAGE

```
AVERAGE [<expression>, ...] [TO <var>, ...] [<scope>]
```

Like `SUM`, but averages.

### TOTAL

```
TOTAL ON <key> TO <file> [FIELDS <field>, ...] [<scope>]
```

Writes a table with one record per key value, summing the numeric fields
listed. The table must be in key order (indexed or sorted).

```
TOTAL ON custno TO custtot FIELDS amount
```

---

## Indexes

### INDEX ON

```
INDEX ON <key> TO <file> [FOR <condition>] [UNIQUE] [DESCENDING]
```

Creates a Clipper NTX index and makes it the controlling index. `UNIQUE`
keeps only the first record for each key; `FOR` indexes only matching
records.

```
INDEX ON Upper(name) TO custname
INDEX ON DToS(orderdate) + custno TO orddate FOR !Deleted()
```

### SET INDEX

```
SET INDEX TO [<file>, ...] [ADDITIVE]
```

Opens indexes for the current table; the first one controls the order.
`SET INDEX TO` with no files closes them. `ADDITIVE` keeps the indexes
already open.

### SET ORDER

```
SET ORDER TO [<n>]
```

Chooses which open index controls the order. `0` (or no number) uses
record-number order.

### REINDEX

Rebuilds all open indexes of the current table.

---

## Filters and relations

### SET FILTER

```
SET FILTER TO [<condition>]
```

Hides records that don't match the condition. `SET FILTER TO` with nothing
removes the filter. Takes effect when the record pointer next moves; use
`GO TOP` afterwards.

```
SET FILTER TO state = "MA" .AND. balance > 0
GO TOP
```

### SET RELATION

```
SET RELATION TO <key> INTO <alias> [, [TO] <key> INTO <alias>...] [ADDITIVE]
SET RELATION TO
```

Links child tables to the current one. Whenever the parent moves, each child
moves to the matching record: by `SEEK` on its controlling index, or by
record number when the key is numeric and the child has no index.

```
SELECT 2
USE orders INDEX ordcust
SELECT 1
USE customer
SET RELATION TO custno INTO orders
LIST name, orders->amount
```

---

## Copying, importing and sorting

### COPY TO

```
COPY TO <file> [FIELDS <field>, ...] [<scope>]
    [SDF | DELIMITED [WITH <delimiter> | WITH BLANK | WITH TAB | WITH PIPE]]
```

Copies records to a new table, or to a text file with `SDF` (fixed width)
or `DELIMITED` (comma-separated, character fields in quotes).

| Clause | Separator | Character fields |
|---|---|---|
| `DELIMITED` | `,` | `"quoted"` |
| `DELIMITED WITH <c>` | `,` | quoted with `<c>` |
| `DELIMITED WITH BLANK` | space | not quoted |
| `DELIMITED WITH TAB` | tab | `"quoted"` |
| `DELIMITED WITH PIPE` | `\|` | `"quoted"` |

```
COPY TO active FOR status = "A"
COPY TO export.txt DELIMITED
```

### SORT

```
SORT TO <file> ON <field> [/A] [/D] [/C] [, <field> [/A] [/D] [/C]...] [<scope>]
```

Writes a sorted copy of the table. `/A` ascending (default), `/D`
descending, `/C` ignore case.

```
SORT TO bycity ON state, city /D
```

### COPY FILE

```
COPY FILE <source> TO <destination>
```

Copies any file.

---

## Files and directories

### CD, CHDIR

```
CD [<directory>]
CHDIR [<directory>]
```

Changes the current directory, which is where `USE`, `DO`, `DIR` and the
other file commands look. With no directory, shows the current one.
`CD -` goes back to the previous directory, and `~` and `$VAR` are
expanded. Quotes are optional, even for names with spaces. An error is
raised if the directory does not exist. `!cd <dir>` and `RUN cd <dir>` do
the same thing (a shell's `cd` would only last as long as that shell).
The status bar shows the current directory.

### DIR, DIRECTORY

```
DIR [<skeleton>]
```

With no skeleton, lists the tables in the current directory with their
record counts, last update and size. With a skeleton such as `*.prg`, lists
matching files.

### TYPE

```
TYPE <file> [TO PRINTER]
```

Prints a text file.

### DELETE FILE, ERASE

```
DELETE FILE <file>
ERASE <file>
```

Deletes a file.

### RENAME

```
RENAME <old name> TO <new name>
```

### MODIFY COMMAND

```
MODIFY COMMAND [<file>]
```

Opens a program in the full-screen editor; asks for a name if none is
given. Saving compile-checks the program.

### MODIFY FILE

```
MODIFY FILE <file>
```

Edits any text file.

---

## Testing

### ASSERT

```
ASSERT <condition> [MESSAGE <text>]
```

Checks that a condition is true. Under `nantucket test`, a failure is
reported with the file and line and the test carries on. In an ordinary
run, a failure stops the program with the error `Assertion failed: <text>`.
Without `MESSAGE`, the condition's own text is used.

```
ASSERT RecCount() == 3
ASSERT !Empty(cName) MESSAGE "name is required"
```

The functions `AssertTrue()`, `AssertFalse()`, `AssertEquals()` and
`AssertError()` do the same for other kinds of check. See
[DOCUMENTATION.md, Testing programs](DOCUMENTATION.md#15-testing-programs).

### nantucket test

```
nantucket test [-run <pattern>] [-update] [-v] [<file or directory>...]
```

Run from the shell, not the dot prompt. Runs every `*_test.prg` file, treats
each `PROCEDURE Test...` as a test, and compares the output with
`<name>_test.out` when that file exists. Exits with 1 if any test fails.

| Option | Effect |
|---|---|
| `-run <pattern>` | Only run test files or `Test` procedures whose name matches (a regular expression, case-insensitive) |
| `-update` | Write the actual output to the `.out` files |
| `-v` | List every test and show each file's output |

---

## SET commands

On/off settings accept `ON`, `OFF`, a macro (`SET EXACT &cMode`) or a
logical expression in parentheses (`SET EXACT (lExact)`). Every setting can
also be read and changed with the `Set()` function and the `_SET_*`
constants from `set.ch`.

### On/off settings

| Command | Default | Effect when ON |
|---|---|---|
| `SET ALTERNATE ON \| OFF` | OFF | Console output also goes to the alternate file |
| `SET BELL ON \| OFF` | OFF | Beep during `READ` when a GET fills up or input is rejected |
| `SET CENTURY ON \| OFF` | OFF | Dates show four-digit years |
| `SET CONFIRM ON \| OFF` | OFF | Enter is needed to leave a full GET |
| `SET CONSOLE ON \| OFF` | ON | `?` output is shown on screen |
| `SET CURSOR ON \| OFF` | ON | The cursor is visible |
| `SET DEBUG ON \| OFF` | OFF | Accepted, no effect |
| `SET DELETED ON \| OFF` | OFF | Records marked for deletion are hidden |
| `SET DELIMITERS ON \| OFF` | OFF | GETs are shown between delimiter characters |
| `SET DOHISTORY ON \| OFF` | OFF | Accepted, no effect |
| `SET ECHO ON \| OFF` | OFF | Show each program line as it runs |
| `SET ESCAPE ON \| OFF` | ON | Esc ends `READ` (when OFF, Esc is ignored) |
| `SET EXACT ON \| OFF` | OFF | `=` compares strings exactly |
| `SET EXCLUSIVE ON \| OFF` | ON | ON: `USE` opens tables for this session alone. OFF: tables are shared |
| `SET FIXED ON \| OFF` | OFF | Numbers always show `SET DECIMALS` decimals |
| `SET HEADING ON \| OFF` | ON | `LIST`/`DISPLAY` print column headings |
| `SET INSERT ON \| OFF` | OFF | GETs start in insert mode |
| `SET INTENSITY ON \| OFF` | ON | GETs use the enhanced/unselected colours (OFF: standard colour) |
| `SET MENU ON \| OFF` | OFF | Accepted, no effect |
| `SET PRINTER ON \| OFF` | OFF | Console output also goes to the printer file (`SET PRINT` works too) |
| `SET SAFETY ON \| OFF` | ON | Ask before overwriting or zapping files |
| `SET SCOREBOARD ON \| OFF` | ON | "Ins" and READ messages appear on row 0 |
| `SET SOFTSEEK ON \| OFF` | OFF | A failed `SEEK` stops at the next higher key |
| `SET STATUS ON \| OFF` | ON at the dot prompt | Status bar on the bottom line: work area, table, record / total records, index, filter, directory, date and time |
| `SET STEP ON \| OFF` | OFF | Show each program line and wait for a key before running it (Esc stops) |
| `SET TALK ON \| OFF` | OFF | Accepted, no effect |
| `SET TRUSTED ON \| OFF` | OFF | ON: index keys and report/label forms may run any expression (see [Expressions in data files](DOCUMENTATION.md#expressions-in-data-files)) |
| `SET UNIQUE ON \| OFF` | OFF | New indexes keep one record per key |
| `SET WRAP ON \| OFF` | OFF | Menu highlights wrap from the last item to the first |

### Settings with values

| Command | Default | Effect |
|---|---|---|
| `SET ALTERNATE TO [<file>] [ADDITIVE]` | none | File for alternate output (`.txt`). `ADDITIVE` appends |
| `SET COLOR TO [<spec>]` | from the theme | Screen colours. No spec: back to the theme's colours |
| `SET COLOUR TO [<spec>]` | | Same as `SET COLOR` |
| `SET DATE [TO] <format>` | AMERICAN | Date format (see below) |
| `SET DATE FORMAT [TO] <string>` | | Custom date format, e.g. `"yyyy-mm-dd"` |
| `SET DECIMALS TO [<n>]` | 2 | Decimals shown for calculated numbers |
| `SET DEFAULT TO [<directory>]` | none | Directory for new files, searched first |
| `SET DELIMITERS TO [<chars> \| DEFAULT]` | `::` | Characters around GETs |
| `SET DEVICE TO SCREEN \| PRINTER` | SCREEN | Where `@ ... SAY` goes |
| `SET EPOCH TO <year>` | 1950 | First year of the century window for two-digit years |
| `SET FORMAT TO [<file>]` | | Accepted, no effect |
| `SET MARGIN TO [<n>]` | 0 | Left margin for printer output |
| `SET MEMOWIDTH TO <n>` | 50 | Wrap width for memos in `?`, `LIST` and `DISPLAY` |
| `SET MESSAGE TO [<row> [CENTER \| CENTRE]]` | 0 (off) | Row (and centring) for menu `MESSAGE` text |
| `SET PATH TO [<directory list>]` | none | Extra directories to search, separated by `;` or `,` |
| `SET PRINTER TO [<file>] [ADDITIVE]` | none | Printer output file (default `nantucket.prn`) |
| `SET THEME TO [<name>]` | startup theme | `color`, `classic` or `mono`; resets colours and clears the screen |
| `SET TYPEAHEAD TO <n>` | 50 | Size of the keyboard buffer |
| `SET CODEPAGE TO [<name>]` | AUTO | Code page for tables that don't name one, and for new tables: `437`, `850`, `852`, `865`, `866`, `1250`, `1251`, `1252`, `UTF8` or `AUTO` (see [Code pages](DOCUMENTATION.md#code-pages)) |

### SET DATE formats

| Command | Format |
|---|---|
| `SET DATE AMERICAN` / `MDY` | mm/dd/yy |
| `SET DATE ANSI` | yy.mm.dd |
| `SET DATE BRITISH` / `FRENCH` / `DMY` | dd/mm/yy |
| `SET DATE GERMAN` | dd.mm.yy |
| `SET DATE ITALIAN` | dd-mm-yy |
| `SET DATE JAPANESE` / `YMD` | yy/mm/dd |
| `SET DATE USA` | mm-dd-yy |

With `SET CENTURY ON` the year has four digits.

### SET COLOR

```
SET COLOR TO [<standard>[, <enhanced>[, <border>[, <background>[, <unselected>]]]]]
SET COLOR TO (<expression>)
```

Each colour is `foreground/background` using `N B G BG R RB GR W`, with `+`
for bright and `*` for blinking. Slots you leave out keep the theme's
colours. See [DOCUMENTATION.md, Colours and themes](DOCUMENTATION.md#10-colours-and-themes).

```
SET COLOR TO W+/B, N/W, , , W/BG
SET COLOR TO                      && back to the theme
```

### SET KEY, SET FUNCTION, SET PROCEDURE, SET INDEX, SET ORDER, SET FILTER, SET RELATION

See [SET KEY](#set-key), [SET FUNCTION](#set-function),
[SET PROCEDURE](#set-procedure), [SET INDEX](#set-index),
[SET ORDER](#set-order), [SET FILTER](#set-filter) and
[SET RELATION](#set-relation).

---

## Dot prompt only

These work at the interactive dot prompt; most also work in programs.

| Command | Description |
|---|---|
| `HELP` | Open a summary of the commands in a window that stays on screen |
| `VERSION` | Show the version number |
| `VERSION FULL` | Open a window showing the version and what's new in it |
| `!<command>` | Run a shell command (`!` alone opens a shell). `!cd <dir>` changes Nantucket's own directory |
| `CD [<dir>]` | Change or show the current directory |
| `RUN <command>` | Same as `!` |
| `QUIT` | Leave Nantucket |
| Up / Down | Recall earlier lines |

Control structures (`IF`, `DO WHILE`, `FOR`, ...) can be typed over several
lines; the prompt runs them once they are closed.

---

## Preprocessor directives

| Directive | Description |
|---|---|
| `#define <name> [<value>]` | Define a constant |
| `#define <name>(<params>) <expression>` | Define a function-like macro |
| `#undef <name>` | Remove a definition |
| `#ifdef <name>` / `#ifndef <name>` | Compile the following lines only if the name is (not) defined |
| `#else` / `#endif` | End or switch a conditional block |
| `#include "<file>"` | Include a header file |
| `#command <pattern> => <result>` | Define a command (keywords may be abbreviated to 4 letters) |
| `#xcommand <pattern> => <result>` | Define a command (keywords must be written in full) |
| `#translate <pattern> => <result>` | Define a translation that can appear anywhere in a line |
| `#xtranslate <pattern> => <result>` | Like `#translate`, without abbreviations |

```
#define VAT_RATE 0.2
#command SHOW <x> => QOut( "Value:", <x> )

SHOW 100 * VAT_RATE
```

See [DOCUMENTATION.md, The preprocessor](DOCUMENTATION.md#7-the-preprocessor)
for the pattern markers.

---

## Alphabetical index

| Command | Section |
|---|---|
| `!` | [RUN, !](#run-) |
| `?`, `??` | [? and ??](#-and-) |
| `@ ... BOX` | [@ ... BOX](#--box) |
| `@ ... CLEAR` | [@ ... CLEAR](#--clear) |
| `@ ... GET` | [@ ... GET](#--get) |
| `@ ... PROMPT` | [@ ... PROMPT and MENU TO](#--prompt-and-menu-to) |
| `@ ... SAY` | [@ ... SAY](#--say) |
| `@ ... TO` | [@ ... TO](#--to) |
| `ACCEPT` | [ACCEPT](#accept) |
| `ANNOUNCE` | [REQUEST, ANNOUNCE, EXTERNAL](#request-announce-external) |
| `ASSERT` | [ASSERT](#assert) |
| `APPEND` | [APPEND](#append) |
| `APPEND BLANK` | [APPEND BLANK](#append-blank) |
| `APPEND FROM` | [APPEND FROM](#append-from) |
| `AVERAGE` | [AVERAGE](#average) |
| `BEGIN SEQUENCE` | [BEGIN SEQUENCE](#begin-sequence) |
| `BREAK` | [BREAK](#break) |
| `BROWSE` | [BROWSE](#browse) |
| `CANCEL` | [CANCEL](#cancel) |
| `CD`, `CHDIR` | [CD, CHDIR](#cd-chdir) |
| `CHANGE` | [EDIT, CHANGE](#edit-change) |
| `CLEAR`, `CLS` | [CLEAR, CLS](#clear-cls) |
| `CLEAR ALL` | [CLEAR ALL](#clear-all) |
| `CLEAR GETS` | [CLEAR GETS](#clear-gets) |
| `CLEAR MEMORY` | [CLEAR MEMORY](#clear-memory) |
| `CLEAR TYPEAHEAD` | [CLEAR TYPEAHEAD](#clear-typeahead) |
| `CLOSE` | [CLOSE](#close) |
| `COMMIT` | [COMMIT](#commit) |
| `COMPILE` | [COMPILE](#compile) |
| `CONTINUE` | [CONTINUE](#continue) |
| `COPY FILE` | [COPY FILE](#copy-file) |
| `COPY STRUCTURE` | [COPY STRUCTURE](#copy-structure) |
| `COPY TO` | [COPY TO](#copy-to) |
| `COUNT` | [COUNT](#count) |
| `CREATE` | [CREATE](#create) |
| `CREATE LABEL` | [CREATE LABEL, MODIFY LABEL](#create-label-modify-label) |
| `CREATE REPORT` | [CREATE REPORT, MODIFY REPORT](#create-report-modify-report) |
| `DECLARE`, `DIMENSION` | [DECLARE, DIMENSION](#declare-dimension) |
| `DEFAULT` | [DEFAULT](#default) |
| `DELETE` | [DELETE](#delete) |
| `DELETE FILE` | [DELETE FILE, ERASE](#delete-file-erase) |
| `DIR`, `DIRECTORY` | [DIR, DIRECTORY](#dir-directory) |
| `DISPLAY` | [LIST, DISPLAY](#list-display) |
| `DISPLAY MEMORY` | [DISPLAY MEMORY, LIST MEMORY](#display-memory-list-memory) |
| `DISPLAY STATUS` | [DISPLAY STATUS, LIST STATUS](#display-status-list-status) |
| `DISPLAY STRUCTURE` | [DISPLAY STRUCTURE, LIST STRUCTURE](#display-structure-list-structure) |
| `DO` | [DO](#do) |
| `DO CASE` | [DO CASE](#do-case) |
| `DO WHILE` | [DO WHILE](#do-while) |
| `EDIT` | [EDIT, CHANGE](#edit-change) |
| `EJECT` | [EJECT](#eject) |
| `ERASE` | [DELETE FILE, ERASE](#delete-file-erase) |
| `EXIT`, `LOOP` | [EXIT, LOOP](#exit-loop) |
| `EXTERNAL` | [REQUEST, ANNOUNCE, EXTERNAL](#request-announce-external) |
| `FIELD` | [MEMVAR, FIELD](#memvar-field) |
| `FIND` | [FIND](#find) |
| `FOR ... NEXT` | [FOR ... NEXT](#for--next) |
| `FOR EACH` | [FOR EACH](#for-each) |
| `FUNCTION` | [FUNCTION, PROCEDURE](#function-procedure) |
| `GO`, `GOTO` | [GO, GOTO](#go-goto) |
| `HELP` | [Dot prompt only](#dot-prompt-only) |
| `IF` | [IF](#if) |
| `INDEX ON` | [INDEX ON](#index-on) |
| `INPUT` | [INPUT](#input) |
| `KEYBOARD` | [KEYBOARD](#keyboard) |
| `LABEL FORM` | [LABEL FORM](#label-form) |
| `LIST` | [LIST, DISPLAY](#list-display) |
| `LOCAL` | [LOCAL](#local) |
| `LOCATE` | [LOCATE](#locate) |
| `MEMVAR` | [MEMVAR, FIELD](#memvar-field) |
| `MENU TO` | [@ ... PROMPT and MENU TO](#--prompt-and-menu-to) |
| `MODIFY COMMAND` | [MODIFY COMMAND](#modify-command) |
| `MODIFY FILE` | [MODIFY FILE](#modify-file) |
| `MODIFY LABEL` | [CREATE LABEL, MODIFY LABEL](#create-label-modify-label) |
| `MODIFY REPORT` | [CREATE REPORT, MODIFY REPORT](#create-report-modify-report) |
| `MODIFY STRUCTURE` | [MODIFY STRUCTURE](#modify-structure) |
| `NOTE` | [NOTE, comments](#note-comments) |
| `PACK` | [PACK](#pack) |
| `PARAMETERS` | [PARAMETERS](#parameters) |
| `PRIVATE` | [PRIVATE](#private) |
| `PROCEDURE` | [FUNCTION, PROCEDURE](#function-procedure) |
| `PUBLIC` | [PUBLIC](#public) |
| `QUIT` | [QUIT](#quit) |
| `READ` | [READ](#read) |
| `RECALL` | [RECALL](#recall) |
| `REINDEX` | [REINDEX](#reindex) |
| `RELEASE` | [RELEASE](#release) |
| `RENAME` | [RENAME](#rename) |
| `REPLACE` | [REPLACE](#replace) |
| `REPORT FORM` | [REPORT FORM](#report-form) |
| `REQUEST` | [REQUEST, ANNOUNCE, EXTERNAL](#request-announce-external) |
| `RESTORE FROM` | [RESTORE](#restore) |
| `RETURN` | [RETURN](#return) |
| `RETURN TO MASTER` | [RETURN TO MASTER](#return-to-master) |
| `RUN` | [RUN, !](#run-) |
| `SAVE TO` | [SAVE](#save) |
| `SEEK` | [SEEK](#seek) |
| `SELECT` | [SELECT](#select) |
| `SET ...` | [SET commands](#set-commands) |
| `SET WINDOW TO` | [SET WINDOW TO](#set-window-to) |
| `SKIP` | [SKIP](#skip) |
| `SORT` | [SORT](#sort) |
| `STATIC` | [STATIC](#static) |
| `STORE` | [STORE](#store) |
| `SUM` | [SUM](#sum) |
| `SWITCH` | [SWITCH](#switch) |
| `TEXT` | [TEXT ... ENDTEXT](#text--endtext) |
| `TOTAL` | [TOTAL](#total) |
| `TRY` | [TRY ... CATCH](#try--catch) |
| `TYPE` | [TYPE](#type) |
| `UNLOCK` | [UNLOCK](#unlock) |
| `USE` | [USE](#use) |
| `VERSION [FULL]` | [Dot prompt only](#dot-prompt-only) |
| `WAIT` | [WAIT](#wait) |
| `WINDOW CLOSE` | [WINDOW CLOSE](#window-close) |
| `WINDOW MAXIMIZE`, `MINIMIZE`, `RESTORE` | [WINDOW MINIMIZE, MAXIMIZE, RESTORE](#window-minimize-maximize-restore) |
| `WINDOW MOVE`, `WINDOW SIZE` | [WINDOW MOVE, WINDOW SIZE](#window-move-window-size) |
| `WINDOW OPEN` | [WINDOW OPEN](#window-open) |
| `ZAP` | [ZAP](#zap) |

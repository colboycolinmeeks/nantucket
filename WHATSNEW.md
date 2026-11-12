# What's New in Nantucket

## 0.2

- New default colours: white text on blue, with GETs in white on green.
- Colours now use the classic VGA palette, so they look the same (and stay
  readable) whatever colour theme your terminal uses. Set
  NANTUCKET_PALETTE=terminal to use the terminal's own colours instead.
- Status bar on the bottom line of the dot prompt, showing the current work
  area, the table in use, the record number and total records, the
  controlling index, whether a filter is set, the current directory, and
  the date and time (the clock keeps ticking while you type).
  SET STATUS OFF hides it; SET STATUS ON brings it back.
- Windows: WINDOW OPEN (or WOpen()) creates a numbered window with a title
  bar and minimise, maximise and close controls. SET WINDOW TO <n> sends
  all screen commands (?, @ SAY/GET, CLS, MaxRow()...) to that window;
  SET WINDOW TO goes back to the full screen (window 0). Each window keeps
  its own SET COLOR. Windows are sizable unless opened FIXED.
- Mouse: drag a title bar to move a window, drag its corner to resize it,
  click the controls to minimise, maximise or close, and click a window to
  bring it to the front.
- HELP opens in a window that stays on screen while you work. Scroll it
  with the mouse wheel.
- VERSION command: shows the version number. VERSION FULL opens a window
  with this list of what's new, which stays on screen like HELP.
- examples/demo.prg: a guided tour of the essentials (run
  nantucket examples/demo, or make demo).
- CD <dir> (or CHDIR) changes the current directory; CD on its own shows
  it and CD - goes back. !cd <dir> and RUN cd <dir> now change
  Nantucket's directory too, instead of only a short-lived shell's.
- Fixed: a variable named after a command, such as run := 1, was taken
  as the command.
- Shared tables: USE ... SHARED with real record and file locks that other
  sessions see (RLock(), FLock(), dbRLock(), UNLOCK, dbRLockList(),
  IsLocked()). USE sets NetErr() when a table is in use elsewhere, instead
  of two sessions silently overwriting each other's records. Changing a
  shared record without a lock raises "Lock required"; PACK, ZAP and
  REINDEX need exclusive use. BROWSE and EDIT lock records for you.
- nantucket build: package a program and the files it uses as a single
  executable.
- Much faster appends to indexed tables: 100,000 records now take under
  half a second instead of 18 seconds.
- Indexes can no longer silently miss records. Closing the terminal or
  kill now saves everything first, and an index that wasn't saved (after
  kill -9 or a power cut) or that missed records added while it was closed
  is rebuilt automatically when opened.
- Fixed: USE ... INDEX and SET INDEX TO now go to the first record in
  index order, so a DO WHILE !Eof() loop no longer skips records.
- REPORT FORM and LABEL FORM print dBase III / Clipper .FRM and .LBL
  report and label forms.
- Debugger: press Alt-D while a program runs (or call AltD(), or start
  it with nantucket -debug) to see its source, call stack and variables.
  Step through it line by line, set breakpoints on lines and procedures
  (conditional too: BP 12 IF nTotal > 1000, or on a hit count: BP 12
  HIT 50, HIT >= 50, HIT % 10), log messages without stopping
  (logpoints: LP 12 "total is {nTotal}"; F3 shows the log), stop when a
  field changes in any record (data breakpoints: DB customer->balance),
  stop when a variable
  changes (watchpoints: WP nTotal), watch expressions, and look at or change any variable, LOCALs
  included. See "Debugging a program" in DOCUMENTATION.md.
- Report and label designers: CREATE REPORT (or MODIFY REPORT) and CREATE
  LABEL (or MODIFY LABEL) make and change .FRM and .LBL forms full
  screen. CREATE REPORT <file> FROM <fields> makes a simple report from
  fields of the current table in one step.
- INIT PROCEDURE and EXIT PROCEDURE now run; TEXT TO FILE and TEXT TO
  PRINTER write the text.
- Program editor: undo (Ctrl-Z or Ctrl-U), Shift+arrow selection with copy, cut and
  paste (Ctrl-Ins, Shift-Del, Shift-Ins), and WordStar / dBase Ctrl-K
  blocks. Saving a program with an error keeps the editor open on the
  faulty line.
- COMMIT and shared tables no longer rewrite whole index files: changes
  are logged, which is many times faster on large tables.
- Damaged tables, memo and .mem files are reported clearly instead of
  causing internal errors.
- Dot prompt history is kept between sessions (~/.nantucket_history).
- Tracing: nantucket -trace <file>, SET ECHO ON and SET STEP ON.
- The interpreter runs arithmetic-heavy loops nearly three times faster.
- Dependencies updated to fix known vulnerabilities; Go 1.25 or later is
  now needed to build Nantucket. GitHub Actions now checks every push.
- Tidier start-up banner.
- Accented letters in DBF tables: text is stored in the table's code page
  (437 by default, as DOS dBase and Clipper used), so other programs read
  it correctly, and old DOS tables show their accented letters. SET
  CODEPAGE TO chooses another code page; tables that name one in their
  header always use it. See "Code pages" in DOCUMENTATION.md.
- Index files and report and label forms can no longer run code hidden in
  their expressions (such as an index key that deletes files). SET
  TRUSTED ON allows it for files you trust.
- Windows without a mouse: F1 opens or closes HELP, Esc on an empty line
  closes the HELP or VERSION FULL window, and F6 arranges windows with
  the keyboard (arrows move, Shift+arrows resize, Tab switches).
- "Lock required" and "Exclusive required" errors now say how to fix them.
- Faster shared tables: moving through a shared table re-reads its header
  less often.
- Indexes changed by another program while Nantucket had them open are
  rebuilt instead of trusting an out-of-date change log.
- Names built with a macro, such as opt&sub (OPT1 when sub is "1"), now
  work on both sides of an assignment, as in dBase and Clipper.
- Old DOS program files that end with a Ctrl-Z end-of-file marker now
  compile; the marker and anything after it are ignored.
- Release archives include THIRD_PARTY_LICENSES, with the copyright
  notices and licences of the libraries Nantucket uses. Include it, with
  LICENSE and NOTICE, when you give others a program made with
  nantucket build.

## 0.1

- First release: dot prompt, DBF/NTX/DBT tables, Clipper language,
  BROWSE/EDIT, MODIFY COMMAND, themes (SET THEME TO classic|color|mono)
  and the test runner (nantucket test).

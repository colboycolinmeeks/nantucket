* demo.prg - a guided tour of Nantucket
*
* Run with:  nantucket examples/demo
*
* The tour walks through the essentials: creating a table, adding and
* editing records, indexes and searching, queries, BROWSE, data entry
* forms, menus and windows. A Guide window explains each step and a Code
* window shows the xBase that does the work, so you can try it yourself
* at the dot prompt afterwards.
*
* Press any key to move on, or Esc at a "Press any key" prompt to leave
* the tour. Run from the dot prompt (DO demo), it returns there at the
* end, with the settings it changed put back. The demo works on a table
* called DEMOCUST.DBF in the current directory, and offers to delete it at
* the end.
#include "inkey.ch"
#include "set.ch"

#define STEPS  9
#define TABLE  "democust"

STATIC nGuide := 0     // the Guide window
STATIC nCode  := 0     // the Code window
STATIC nStep  := 0     // current step
STATIC lQuit  := .F.   // Esc pressed: leave the tour

LOCAL i, aSaved
LOCAL aSteps := { {|| StepCreate()}, {|| StepAppend()}, {|| StepIndex()}, ;
                  {|| StepQuery()},  {|| StepBrowse()}, {|| StepForm()},  ;
                  {|| StepEdit()},   {|| StepMenus()},  {|| StepWindows()} }

// the tour can be run more than once in a session (DO demo)
nGuide := 0
nCode  := 0
nStep  := 0
lQuit  := .F.

IF MaxRow() < 23 .OR. MaxCol() < 79
   ? "The demo needs a screen of at least 80 x 24."
   RETURN
ENDIF

// remember the settings the tour changes, to put them back at the end
aSaved := { Set(_SET_DATEFORMAT), Set(_SET_CENTURY), Set(_SET_CONFIRM), ;
            Set(_SET_SCOREBOARD), Set(_SET_CURSOR) }
SET DATE BRITISH
SET CENTURY ON
SET CONFIRM ON
SET SCOREBOARD OFF
SET CURSOR OFF

Welcome()
FOR i := 1 TO Len(aSteps)
   IF lQuit
      EXIT
   ENDIF
   Eval(aSteps[i])
NEXT
Finale()

// back to where the tour was started from, as it was
Set(_SET_CENTURY, aSaved[2])
Set(_SET_DATEFORMAT, aSaved[1])
Set(_SET_CONFIRM, aSaved[3])
Set(_SET_SCOREBOARD, aSaved[4])
Set(_SET_CURSOR, aSaved[5])
RETURN

* ---------------------------------------------------------------------
* The steps
* ---------------------------------------------------------------------

STATIC PROCEDURE Welcome()
   LOCAL nMid := Int(MaxCol() / 2)
   Backdrop("Welcome")
   @  4, nMid - 30 TO 14, nMid + 30 DOUBLE
   @  6, nMid - 28 SAY PadC("N A N T U C K E T   " + SubStr(Version(), 11), 57) COLOR "GR+/B"
   @  8, nMid - 28 SAY PadC("A dBase III / Clipper compatible environment", 57)
   @ 10, nMid - 28 SAY PadC("This tour shows the essentials in nine short steps:", 57)
   @ 11, nMid - 28 SAY PadC("tables, records, indexes, queries, BROWSE, forms,", 57)
   @ 12, nMid - 28 SAY PadC("EDIT, menus and windows.", 57)
   IF Alert("Ready for the tour?", {"Start", "Not now"}) != 1
      lQuit := .T.
   ENDIF
RETURN

STATIC PROCEDURE StepCreate()
   Backdrop("Creating a table")
   Code({ ;
      'dbCreate("democust", {;', ;
      '   {"NAME",    "C", 20, 0},;', ;
      '   {"CITY",    "C", 14, 0},;', ;
      '   {"STATE",   "C",  2, 0},;', ;
      '   {"BALANCE", "N", 10, 2},;', ;
      '   {"JOINED",  "D",  8, 0},;', ;
      '   {"ACTIVE",  "L",  1, 0},;', ;
      '   {"NOTES",   "M", 10, 0} })', ;
      'USE democust', ;
      'DISPLAY STRUCTURE' })
   Guide({ ;
      "A table (a .DBF file) is made from a list of fields: a name, a type", ;
      "(Character, Numeric, Date, Logical or Memo), a width and decimals.", ;
      "", ;
      "dbCreate() writes the file and USE opens it in the current work area.", ;
      "Memo fields are stored alongside in a .DBT file." })
   IF !Pause()
      RETURN
   ENDIF

   CLOSE DATABASES
   dbCreate(TABLE, { ;
      {"NAME",    "C", 20, 0}, ;
      {"CITY",    "C", 14, 0}, ;
      {"STATE",   "C",  2, 0}, ;
      {"BALANCE", "N", 10, 2}, ;
      {"JOINED",  "D",  8, 0}, ;
      {"ACTIVE",  "L",  1, 0}, ;
      {"NOTES",   "M", 10, 0} })
   USE (TABLE)

   Stage()
   @ 2, 2 SAY "DISPLAY STRUCTURE shows what we made:"
   SetPos(3, 0)
   DISPLAY STRUCTURE
   Guide({ ;
      "The table is created and open. It has no records yet.", ;
      "", ;
      "At the dot prompt you can also type CREATE <file> to design a table", ;
      "interactively, and MODIFY STRUCTURE to change one later." })
   Pause()
RETURN

STATIC PROCEDURE StepAppend()
   LOCAL aData, i, nWide := 50
   aData := { ;
      {"Murphy, Sean",     "Salem",      "MA",  2600.00, "19/05/2018", .T., ""}, ;
      {"Chen, Wei",        "Portland",   "ME",  4200.00, "23/11/2018", .T., "Wholesale account."}, ;
      {"O'Brien, Kate",    "Nantucket",  "MA",  5150.00, "01/06/2015", .T., "Our first customer."}, ;
      {"Abbott, Grace",    "Boston",     "MA",  1250.00, "12/03/2019", .T., "Prefers email."}, ;
      {"Kim, Soo-jin",     "Bangor",     "ME",  1780.40, "03/03/2020", .T., "Seasonal orders."}, ;
      {"Evans, Owen",      "Hartford",   "CT",   875.25, "17/06/2020", .T., ""}, ;
      {"Hughes, Ellen",    "Boston",     "MA",  3100.75, "08/08/2016", .T., "Long standing customer."}, ;
      {"Baker, Tom",       "Nantucket",  "MA",   310.50, "02/07/2021", .T., ""}, ;
      {"Nakamura, Yui",    "Montpelier", "VT",   420.80, "09/09/2021", .T., ""}, ;
      {"Diaz, Maria",      "Providence", "RI",     0.00, "05/01/2022", .F., "Closed in 2023."}, ;
      {"Jones, Peter",     "Worcester",  "MA",   990.00, "11/10/2019", .T., ""}, ;
      {"Fischer, Anna",    "Burlington", "VT",  2300.00, "30/09/2017", .T., "Pays quarterly."}, ;
      {"Lopez, Carlos",    "New Haven",  "CT",    55.00, "27/12/2022", .T., ""}, ;
      {"Gupta, Ravi",      "Concord",    "NH",   150.00, "14/02/2023", .T., ""}, ;
      {"Ivanova, Olga",    "Newport",    "RI",   640.00, "21/04/2021", .F., ""} }

   Backdrop("Adding records")
   Code({ ;
      'APPEND BLANK', ;
      'REPLACE name    WITH "Abbott, Grace",;', ;
      '        city    WITH "Boston",;', ;
      '        state   WITH "MA",;', ;
      '        balance WITH 1250.00,;', ;
      '        joined  WITH CToD("12/03/2019"),;', ;
      '        active  WITH .T.,;', ;
      '        notes   WITH "Prefers email."' })
   Guide({ ;
      "APPEND BLANK adds an empty record at the end of the table and makes", ;
      "it the current one. REPLACE fills in its fields.", ;
      "", ;
      "We'll add " + LTrim(Str(Len(aData))) + " customers this way, from an array in the program." })
   IF !Pause()
      RETURN
   ENDIF

   Stage()
   @ 3, 4 SAY "Adding records..."
   @ 5, 4 SAY "[" + Space(nWide) + "]"
   FOR i := 1 TO Len(aData)
      APPEND BLANK
      REPLACE name    WITH aData[i][1], ;
              city    WITH aData[i][2], ;
              state   WITH aData[i][3], ;
              balance WITH aData[i][4], ;
              joined  WITH CToD(aData[i][5]), ;
              active  WITH aData[i][6], ;
              notes   WITH aData[i][7]
      @ 5, 5 SAY Replicate("█", Int(nWide * i / Len(aData))) COLOR "GR+/B"
      @ 7, 4 SAY PadR(LTrim(Str(RecNo())) + ": " + Trim(name), 40)
      Inkey(0.08)
   NEXT
   @ 7, 4 SAY PadR("Done: RecCount() = " + LTrim(Str(RecCount())), 40)
   @ 8, 4 SAY "GO TOP, then LIST NEXT 6 name, city, balance shows the first few:"
   GO TOP
   SetPos(8, 0)
   LIST NEXT 6 name, city, balance
   Guide({ ;
      "Every customer is in the table. LIST prints records to the screen;", ;
      "here we only listed the first six (LIST NEXT 6).", ;
      "", ;
      "DELETE marks a record for deletion, RECALL brings it back and PACK", ;
      "removes deleted records for good." })
   Pause()
RETURN

STATIC PROCEDURE StepIndex()
   Backdrop("Indexes and searching")
   Code({ ;
      'INDEX ON Upper(name) TO democust', ;
      'SEEK "MURPHY"', ;
      'IF Found()', ;
      '   ? name, city, balance', ;
      'ENDIF', ;
      '', ;
      'LOCATE FOR city = "Nantucket"', ;
      'DO WHILE Found()', ;
      '   ? name', ;
      '   CONTINUE', ;
      'ENDDO' })
   Guide({ ;
      "An index keeps the records in order of a key expression and makes", ;
      "SEEK fast. Indexes are .NTX files, kept up to date as records change.", ;
      "", ;
      "LOCATE searches without an index, one record at a time." })
   IF !Pause()
      RETURN
   ENDIF

   INDEX ON Upper(name) TO (TABLE)
   Stage()
   @ 2, 2 SAY "Records in name order (note the record numbers):"
   SetPos(3, 0)
   LIST NEXT 6 name, state
   SEEK "MURPHY"
   @ 11, 2 SAY 'SEEK "MURPHY"  ->  Found() = ' + iif(Found(), ".T.", ".F.") + ;
               "   record " + LTrim(Str(RecNo())) + ": " + Trim(name) + ", " + Trim(city)
   @ 13, 2 SAY 'LOCATE FOR city = "Nantucket" finds:'
   SET ORDER TO 0
   LOCATE FOR city = "Nantucket"
   DO WHILE Found()
      @ Row() + 1, 6 SAY Trim(name)
      CONTINUE
   ENDDO
   SET ORDER TO 1
   Guide({ ;
      "SEEK jumped straight to Murphy using the index. LOCATE read the table", ;
      "from the top and CONTINUE carried on to the next match.", ;
      "", ;
      "SET ORDER TO 0 reads in record order; SET ORDER TO 1 goes back to", ;
      "the index." })
   Pause()
RETURN

STATIC PROCEDURE StepQuery()
   LOCAL nCount, nTotal, nAvg, nMA
   Backdrop("Queries and totals")
   Code({ ;
      'COUNT TO nCount', ;
      'SUM balance TO nTotal', ;
      'AVERAGE balance TO nAvg FOR active', ;
      'COUNT TO nMA FOR state == "MA"', ;
      '', ;
      'SET FILTER TO balance > 2000', ;
      'LIST name, balance', ;
      'SET FILTER TO' })
   Guide({ ;
      "COUNT, SUM and AVERAGE work through the table, optionally only for", ;
      "records matching FOR or WHILE conditions.", ;
      "", ;
      "SET FILTER hides the records that don't match from every command." })
   IF !Pause()
      RETURN
   ENDIF

   COUNT TO nCount
   SUM balance TO nTotal
   AVERAGE balance TO nAvg FOR active
   COUNT TO nMA FOR state == "MA"
   Stage()
   @ 2, 4 SAY "Customers                 " + Str(nCount, 10)
   @ 3, 4 SAY "Total balance             " + Transform(nTotal, "999,999.99")
   @ 4, 4 SAY "Average balance (active)  " + Transform(nAvg, "999,999.99")
   @ 5, 4 SAY "Customers in MA           " + Str(nMA, 10)
   @ 7, 4 SAY "With SET FILTER TO balance > 2000:"
   SET FILTER TO balance > 2000
   GO TOP
   SetPos(8, 0)
   LIST name, balance
   SET FILTER TO
   GO TOP
   Guide({ ;
      "Transform() formats a value with a PICTURE: 999,999.99 adds the", ;
      "thousands separators.", ;
      "", ;
      "The filter is cleared again with SET FILTER TO on its own." })
   Pause()
RETURN

STATIC PROCEDURE StepBrowse()
   LOCAL nWin
   Backdrop("BROWSE")
   Code({ ;
      'BROWSE', ;
      '', ;
      '* or a chosen set of fields:', ;
      'BROWSE FIELDS name, city, balance', ;
      '', ;
      '* or inside a window:', ;
      'WINDOW OPEN 2, 2 TO 14, 60 TO nWin', ;
      'BROWSE', ;
      'WINDOW CLOSE nWin' })
   Guide({ ;
      "BROWSE shows the table as a grid you can scroll and edit in place.", ;
      "Arrows move, PgUp/PgDn page, Enter edits the field under the cursor,", ;
      "Ctrl-U deletes or recalls a record.", ;
      "", ;
      "Try changing a balance, then press Esc when done." })
   IF !Pause()
      RETURN
   ENDIF

   Stage()
   GO TOP
   nWin := WOpen(1, 1, MaxRow() - 9, MaxCol() - 1, "BROWSE - Esc when done", "W+/B")
   BROWSE FIELDS name, city, state, balance, active
   WClose(nWin)
   Guide({ ;
      "Screen commands work inside whichever window is selected, so the same", ;
      "BROWSE fills a window as easily as the whole screen.", ;
      "", ;
      "Your changes were written straight to the table." })
   Pause()
RETURN

STATIC PROCEDURE StepForm()
   LOCAL nWin
   LOCAL cName := PadR("Quinn, Robin", 20), cCity := PadR("Nantucket", 14)
   LOCAL cState := "MA", nBalance := 500, dJoined := Date(), lActive := .T.
   Backdrop("A data entry form")
   Code({ ;
      '@ 2, 2 SAY "Name   " GET cName PICTURE "@!";', ;
      '        VALID !Empty(cName)', ;
      '@ 3, 2 SAY "State  " GET cState PICTURE "!!"', ;
      '@ 4, 2 SAY "Balance" GET nBalance;', ;
      '        PICTURE "999,999.99" RANGE 0, 999999', ;
      '@ 5, 2 SAY "Joined " GET dJoined', ;
      'READ', ;
      'IF LastKey() != K_ESC', ;
      '   APPEND BLANK', ;
      '   REPLACE name WITH cName, ...', ;
      'ENDIF' })
   Guide({ ;
      "@ ... SAY ... GET puts a field on screen and READ lets the user fill", ;
      "in the GETs (shown in white on green). PICTURE formats and checks the", ;
      "input, VALID and RANGE reject bad values.", ;
      "", ;
      "Fill in a new customer (or just press Enter through it)." })
   IF !Pause()
      RETURN
   ENDIF

   nWin := WOpen(2, 4, 12, 52, "New customer", "W+/B", .F.)
   SET CURSOR ON
   @ 1, 2 SAY "Name    " GET cName    PICTURE "@!" VALID !Empty(cName)
   @ 2, 2 SAY "City    " GET cCity
   @ 3, 2 SAY "State   " GET cState   PICTURE "!!"
   @ 4, 2 SAY "Balance " GET nBalance PICTURE "999,999.99" RANGE 0, 999999
   @ 5, 2 SAY "Joined  " GET dJoined
   @ 6, 2 SAY "Active  " GET lActive  PICTURE "Y"
   @ 8, 2 SAY "Enter moves on, Esc cancels"
   READ
   SET CURSOR OFF
   WClose(nWin)
   IF LastKey() != K_ESC
      APPEND BLANK
      REPLACE name WITH cName, city WITH cCity, state WITH cState, ;
              balance WITH nBalance, joined WITH dJoined, active WITH lActive
      Alert("Saved " + Trim(name) + " as record " + LTrim(Str(RecNo())))
   ELSE
      Alert("Cancelled - nothing saved")
   ENDIF
   Guide({ ;
      "That window was opened FIXED (the last argument of WOpen() is .F.),", ;
      "so it can be moved and minimised but not resized.", ;
      "", ;
      "Alert() shows a message box and returns the button chosen." })
   Pause()
RETURN

STATIC PROCEDURE StepEdit()
   Backdrop("EDIT")
   Code({ ;
      'SEEK "QUINN"', ;
      'EDIT' })
   Guide({ ;
      "EDIT shows one record at a time as a form, with every field ready", ;
      "to change. PgDn and PgUp move to the next and previous record.", ;
      "", ;
      "NOTES is a memo field: it shows as Memo when it has text. In BROWSE,", ;
      "Enter on a memo field opens it in an editor. Press Esc when done." })
   IF !Pause()
      RETURN
   ENDIF

   Stage()
   SEEK "QUINN"
   IF !Found()
      GO TOP
   ENDIF
   SET CURSOR ON
   EDIT
   SET CURSOR OFF
   Guide({ ;
      "EDIT and BROWSE are the quickest way to look after a table by hand.", ;
      "At the dot prompt, APPEND opens the same form for new records." })
   Pause()
RETURN

STATIC PROCEDURE StepMenus()
   LOCAL nChoice := 1, aStates := {}, nPick, nRow
   Backdrop("Menus")
   Code({ ;
      '@ 3, 4 PROMPT " By state  "', ;
      '@ 4, 4 PROMPT " Top three "', ;
      'MENU TO nChoice', ;
      '', ;
      'nPick := AChoice(3, 30, 10, 40, aStates)' })
   Guide({ ;
      "@ ... PROMPT and MENU TO make a light bar menu; AChoice() picks from", ;
      "a scrolling list. Arrows move, Enter chooses.", ;
      "", ;
      "Choose a report, then a state." })
   IF !Pause()
      RETURN
   ENDIF

   Stage()
   @ 2, 4 SAY "Reports"
   @ 3, 4 PROMPT " By state  " MESSAGE ""
   @ 4, 4 PROMPT " Top three " MESSAGE ""
   MENU TO nChoice
   IF nChoice == 2
      INDEX ON Descend(balance) TO demotop
      GO TOP
      nRow := 7
      @ 6, 4 SAY "Top three balances:"
      DO WHILE !Eof() .AND. nRow < 10
         @ nRow, 6 SAY name + Transform(balance, "999,999.99")
         nRow++
         SKIP
      ENDDO
      SET INDEX TO (TABLE)
   ELSE
      GO TOP
      DO WHILE !Eof()
         IF AScan(aStates, state) == 0
            AAdd(aStates, state)
         ENDIF
         SKIP
      ENDDO
      ASort(aStates)
      @ 2, 30 SAY "State"
      @ 3, 29 TO 3 + Len(aStates) + 1, 36
      nPick := AChoice(4, 31, 3 + Len(aStates), 34, aStates)
      IF nPick > 0
         @ 2, 42 SAY "Customers in " + aStates[nPick] + ":"
         nRow := 3
         GO TOP
         DO WHILE !Eof()
            IF state == aStates[nPick]
               @ nRow, 42 SAY Trim(name)
               nRow++
            ENDIF
            SKIP
         ENDDO
      ENDIF
   ENDIF
   IF nChoice == 2
      Guide({ ;
         "Descend() made an index in reverse order of balance, so the largest", ;
         "came first. Any expression can be an index key." })
   ELSE
      Guide({ ;
         "The list of states was built with AScan() and AAdd() while reading", ;
         "the table, then sorted with ASort(). Arrays grow as needed." })
   ENDIF
   Pause()
RETURN

STATIC PROCEDURE StepWindows()
   LOCAL aWin := {}, i, nW, nTop, nLeft, nRow
   LOCAL aColors := {"W+/B", "N/BG", "W+/R", "N/W"}
   LOCAL aNames := {"Boston", "Nantucket", "Portland", "Hartford"}
   Backdrop("Windows")
   Code({ ;
      'WINDOW OPEN 2, 2 TO 9, 30 TITLE "Boston";', ;
      '            COLOR "N/BG" TO nWin', ;
      '? "Every screen command now"', ;
      '? "works inside window", nWin', ;
      'SET WINDOW TO 0      && full screen', ;
      'SET WINDOW TO nWin   && back again', ;
      'WMove(5, 20)', ;
      'WINDOW MAXIMIZE', ;
      'WINDOW MINIMIZE', ;
      'WINDOW RESTORE', ;
      'WINDOW CLOSE' })
   Guide({ ;
      "Windows are numbered (the full screen is window 0). The selected one", ;
      "gets all the screen output, in its own coordinates and colours. Each", ;
      "has [_] minimise, [↑] maximise and [x] close controls.", ;
      "", ;
      "Watch: four windows, one per city." })
   IF !Pause()
      RETURN
   ENDIF

   Stage()
   FOR i := 1 TO 4
      nTop  := iif(i <= 2, 2, 9)
      nLeft := iif(i % 2 == 1, 1, 41)
      nW := WOpen(nTop, nLeft, nTop + 6, nLeft + 37, aNames[i], aColors[i])
      AAdd(aWin, nW)
      @ 0, 1 SAY "Window " + LTrim(Str(nW)) + ": MaxRow() " + LTrim(Str(MaxRow())) + ;
                 ", MaxCol() " + LTrim(Str(MaxCol()))
      nRow := 2
      GO TOP
      DO WHILE !Eof()
         IF Trim(city) == aNames[i] .AND. nRow <= MaxRow()
            @ nRow, 1 SAY Trim(name) + "  " + LTrim(Transform(balance, "999,999.99"))
            nRow++
         ENDIF
         SKIP
      ENDDO
      Inkey(0.4)
   NEXT
   SET WINDOW TO 0
   IF !Pause("moving a window")
      CloseAll(aWin)
      RETURN
   ENDIF

   SET WINDOW TO aWin[1]
   FOR i := 1 TO 10
      WMove(WRow() + iif(i % 2 == 0, 1, 0), WCol() + 2)
      Inkey(0.05)
   NEXT
   IF !Pause("maximise and minimise")
      CloseAll(aWin)
      RETURN
   ENDIF

   SET WINDOW TO aWin[2]
   WINDOW MAXIMIZE
   @ 1, 1 SAY "Maximised: MaxRow() " + LTrim(Str(MaxRow())) + ;
              ", MaxCol() " + LTrim(Str(MaxCol()))
   Inkey(1.5)
   WINDOW RESTORE
   Inkey(0.6)
   FOR i := 1 TO 4
      WMinimize(aWin[i])
      Inkey(0.3)
   NEXT
   Guide({ ;
      "Minimised windows become tabs along the bottom of the screen.", ;
      "Click a tab to bring a window back. With the mouse you can also", ;
      "drag a title bar to move a window, drag the ◢ corner to resize it,", ;
      "and click a window to bring it to the front." })
   IF !Pause("your turn")
      CloseAll(aWin)
      RETURN
   ENDIF

   FOR i := 1 TO 4
      WRestore(aWin[i])
      Inkey(0.3)
   NEXT
   SET WINDOW TO 0
   Guide({ ;
      "Your turn: drag the windows around, resize them, try the controls.", ;
      "Typing into a window is just as easy: SET WINDOW TO 2, then ? or", ;
      "@ SAY. HELP at the dot prompt opens a window you can keep open.", ;
      "", ;
      "Press a key when you have finished playing." })
   Pause()
   CloseAll(aWin)
RETURN

STATIC PROCEDURE Finale()
   LOCAL nKeep
   CloseGuide()
   CLOSE DATABASES
   Backdrop("That's the tour")
   @  3, 4 SAY "You have seen:"
   @  5, 6 SAY "dbCreate(), USE, APPEND BLANK, REPLACE, LIST, DISPLAY STRUCTURE"
   @  6, 6 SAY "INDEX ON, SEEK, LOCATE / CONTINUE, SET ORDER, Descend()"
   @  7, 6 SAY "COUNT, SUM, AVERAGE, SET FILTER, Transform()"
   @  8, 6 SAY "BROWSE, EDIT, @ SAY / GET / READ with PICTURE, VALID and RANGE"
   @  9, 6 SAY "Alert(), @ PROMPT / MENU TO, AChoice()"
   @ 10, 6 SAY "WINDOW OPEN, SET WINDOW TO, WMove(), maximise, minimise, restore"
   @ 12, 4 SAY "Next steps:"
   @ 14, 6 SAY "nantucket                 the dot prompt (type HELP for commands)"
   @ 15, 6 SAY "nantucket examples/contacts   a small address book program"
   @ 16, 6 SAY "MODIFY COMMAND myprog     write your own program"
   @ 17, 6 SAY "VERSION FULL              what's new in this release"
   IF File(TABLE + ".dbf")
      nKeep := Alert("Keep the demo table DEMOCUST.DBF?", {"Delete it", "Keep it"})
      IF nKeep == 1
         FErase(TABLE + ".dbf")
         FErase(TABLE + ".dbt")
         FErase(TABLE + ".ntx")
         FErase("demotop.ntx")
      ENDIF
   ENDIF
   SET CURSOR ON
   CLEAR
   ? "Thanks for taking the Nantucket tour."
RETURN

* ---------------------------------------------------------------------
* Helpers
* ---------------------------------------------------------------------

* Backdrop clears the screen and draws the title line for a new step.
STATIC PROCEDURE Backdrop(cTitle)
   LOCAL cHead
   WSelect(0)
   CloseCode()
   CLEAR
   nStep++
   cHead := " Nantucket tour"
   IF nStep > 1
      cHead += "  -  step " + LTrim(Str(nStep - 1)) + " of " + LTrim(Str(STEPS))
   ENDIF
   cHead += "  -  " + cTitle + " "
   @ 0, 0 SAY PadR(cHead, MaxCol() + 1) COLOR "N/BG"
RETURN

* Stage clears the demonstration area (above the Guide window).
STATIC PROCEDURE Stage()
   CloseCode()
   WSelect(0)
   Scroll(1, 0, MaxRow() - 9, MaxCol())
   SetPos(1, 0)
RETURN

* Guide shows text in the Guide window at the bottom of the screen.
STATIC PROCEDURE Guide(aLines)
   LOCAL nOld := WSelect(), i
   CloseGuide()
   WSelect(0)
   nGuide := WOpen(MaxRow() - 8, 0, MaxRow(), MaxCol(), "Guide", "N/W", .F.)
   FOR i := 1 TO Len(aLines)
      @ i - 1, 1 SAY aLines[i]
   NEXT
   WSelect(0)
   IF nOld != 0 .AND. AScan(WList(), nOld) > 0
      WSelect(nOld)
   ENDIF
RETURN

* Code shows the xBase for this step in a window at the top right.
STATIC PROCEDURE Code(aLines)
   LOCAL i, nWide := 0, nOld := WSelect()
   CloseCode()
   AEval(aLines, {|c| nWide := Max(nWide, Len(c))})
   WSelect(0)
   nWide := Max(nWide, 20)
   nCode := WOpen(2, MaxCol() - nWide - 4, 3 + Len(aLines), MaxCol() - 1, "Code", "GR+/N")
   FOR i := 1 TO Len(aLines)
      @ i - 1, 1 SAY aLines[i]
   NEXT
   WSelect(0)
   IF nOld != 0 .AND. AScan(WList(), nOld) > 0
      WSelect(nOld)
   ENDIF
RETURN

STATIC PROCEDURE CloseGuide()
   IF nGuide != 0 .AND. AScan(WList(), nGuide) > 0
      WClose(nGuide)
   ENDIF
   nGuide := 0
RETURN

STATIC PROCEDURE CloseCode()
   IF nCode != 0 .AND. AScan(WList(), nCode) > 0
      WClose(nCode)
   ENDIF
   nCode := 0
RETURN

STATIC PROCEDURE CloseAll(aWin)
   AEval(aWin, {|n| iif(AScan(WList(), n) > 0, WClose(n), NIL)})
   WSelect(0)
RETURN

* Pause waits for a key under the Guide text. Esc leaves the tour.
STATIC FUNCTION Pause(cNext)
   LOCAL nKey, nOld := WSelect(), cMsg
   cMsg := "Any key to continue" + iif(cNext == NIL, "", " - next: " + cNext)
   cMsg := PadR(cMsg, MaxCol() - 22) + "Esc leaves the tour"
   IF nGuide != 0 .AND. AScan(WList(), nGuide) > 0
      WSelect(nGuide)
      @ MaxRow(), 1 SAY PadR(cMsg, MaxCol() - 1) COLOR "W+/N"
   ENDIF
   nKey := Inkey(0)
   IF nGuide != 0 .AND. AScan(WList(), nGuide) > 0
      WSelect(nGuide)
      @ MaxRow(), 1 SAY Space(MaxCol() - 1)
   ENDIF
   IF AScan(WList(), nOld) > 0 .OR. nOld == 0
      WSelect(nOld)
   ENDIF
   IF nKey == K_ESC
      lQuit := .T.
   ENDIF
RETURN !lQuit

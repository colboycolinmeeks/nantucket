* Compatibility features: SEQUENCE WITH, macro calls, memo type,
* SET MEMOWIDTH, SET MARGIN / PRow(), SET TYPEAHEAD with KEYBOARD.
LOCAL x
PRIVATE cFunc := "Upper"

? "-- BEGIN SEQUENCE WITH"
BEGIN SEQUENCE WITH {|e| Break(e)}
   x := 1 / "a"
RECOVER USING x
   ? "recovered:", x:description
END SEQUENCE
BEGIN SEQUENCE WITH {|e| 42}
   x := 1 / "a"
END SEQUENCE
? "substituted:", x

? "-- macro calls"
? &cFunc("abc"), &cFunc.("def"), &(cFunc + "('ghi')")

? "-- macro-built names"
PRIVATE opt1 := "Add", opt2 := "2. Edit", sub
?
FOR sub := 1 TO 2
   sub := Str(sub, 1)
   opt&sub := IIF(Val(opt&sub) = 0, sub + ". " + opt&sub, opt&sub)
   ?? opt&sub, ""
   sub := Val(sub)
NEXT
sub := "x"
opt&sub.y := "dot"
? optxy

? "-- memo type"
dbCreate("memos", {{"NAME", "C", 5, 0}, {"NOTES", "M", 10, 0}})
USE memos
APPEND BLANK
REPLACE name WITH "one", notes WITH "The quick brown fox jumps over the lazy dog"
? ValType(notes), Type("notes"), ValType(Upper(notes)), FieldType(2)
x := notes
? ValType(x)

? "-- SET MEMOWIDTH"
SET MEMOWIDTH TO 20
? notes
LIST name, notes
USE

? "-- SET MARGIN and PRow()"
SET PRINTER TO margin.txt
SET MARGIN TO 4
SET DEVICE TO PRINTER
@ 2, 3 SAY "row two"
? PRow(), PCol()
SET DEVICE TO SCREEN
SET PRINTER TO
? StrTran(MemoRead("margin.txt"), Chr(13) + Chr(10), "|")

? "-- SET TYPEAHEAD"
SET TYPEAHEAD TO 3
KEYBOARD "abcdef"
x := ""
DO WHILE NextKey() != 0
   x += Chr(Inkey())
ENDDO
? x

? "-- SELECT 0 and DELIMITED WITH TAB / PIPE"
CLOSE DATABASES
dbCreate("tabs", {{"NAME", "C", 6, 0}, {"AMT", "N", 4, 0}})
SELECT 0
USE tabs
? Select()
APPEND BLANK
REPLACE name WITH "a, b", amt WITH 7
COPY TO tabs.txt DELIMITED WITH TAB
? StrTran(MemoRead("tabs.txt"), Chr(9), "<tab>")
COPY TO pipe.txt DELIMITED WITH PIPE
? MemoRead("pipe.txt")
ZAP
APPEND FROM tabs.txt DELIMITED WITH TAB
? RecCount(), name, amt

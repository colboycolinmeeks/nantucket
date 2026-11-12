LOCAL i, t := Seconds(), ok := .T., prev, n
dbCreate("big", {{"ID","N",8,0},{"CODE","C",12,0},{"AMT","N",10,2}})
USE big
FOR i := 1 TO 20000
   APPEND BLANK
   REPLACE id WITH i, code WITH "K" + StrZero((i * 7919) % 20000, 6), amt WITH (i % 100) - 50
NEXT
t := Seconds()
INDEX ON code TO bigcode
INDEX ON amt TO bigamt
CLOSE ALL
USE big INDEX bigcode, bigamt
? "keys", OrdKeyCount()
GO TOP
prev := ""
n := 0
DO WHILE !Eof()
   IF code < prev
      ok := .F.
   ENDIF
   prev := code
   n++
   SKIP
ENDDO
? "ordered", ok, n
SEEK "K000777"
? Found(), code, id
REPLACE code WITH "A00000"
GO TOP
? "top now", code, id
SET ORDER TO 2
GO TOP
? "lowest amt", amt
GO BOTTOM
? "highest amt", amt
SEEK -50
? Found(), amt
SEEK -49.5
? Found()
CLOSE ALL
USE big INDEX bigcode
SEEK "A00000"
? "persisted", Found(), id
INDEX ON code TO bigu UNIQUE
? "unique count", OrdKeyCount()

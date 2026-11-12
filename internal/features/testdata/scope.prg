#include "inkey.ch"
#define GREETING "hi there"
#command SHOUT <x> => QOut(Upper(<x>) + "!")
#xtranslate DOUBLE(<n>) => ((<n>) * 2)
STATIC sCount := 100
PUBLIC gPub
LOCAL aBlocks := {}, i, x := 1, y := 2
SHOUT GREETING
? DOUBLE(21), K_ESC
FOR i := 1 TO 3
   AAdd(aBlocks, MakeAdder(i))
NEXT
? Eval(aBlocks[1], 10), Eval(aBlocks[3], 10)
? Counter(), Counter(), sCount
Swap(@x, @y)
? "swapped", x, y
? "pcount", Args(1, 2, 3)
PRIVATE pv := "outer"
ShowPriv()
? "after", pv
gPub := "public!"
SetPub()
? gPub
TEXT
  Text block line 1
  pv = &pv
ENDTEXT
SAVE TO mvars ALL LIKE p*
RELEASE pv
RESTORE FROM mvars ADDITIVE
? "restored", pv
SET PROCEDURE TO lib
? LibHello("x")
bOld := ErrorBlock({|e| Break(e)})
BEGIN SEQUENCE
   ? 1 / 0
RECOVER USING oErr
   ? "Error caught:", oErr:description, oErr:subCode
END
ErrorBlock(bOld)
? IIf(.T., "iif-yes", "iif-no"), If(1 > 2, "a", "b")
? StrTran("a-b-c", "-", "+"), PadL("7", 3, "0"), PadC("hi", 6, "*"), AllTrim("  x  ") + "|"
? At("lo", "hello"), RAt("l", "hello"), Left("hello", 2), Right("hello", 3), Replicate("ab", 3)
? Val("12.50") + 1, Int(7.9), Round(2.345, 2), Abs(-3), Max(3, 9), Min(3, 9), Mod(-7, 3)
? Len(Space(5)), Asc("A"), Chr(66), "abc" $ "xabcx", Empty(""), Empty(0), Empty(CToD(""))
a := {5, 3, 9, 1}
ASort(a)
? a[1], a[4], AScan(a, 9), ATail(a)
ASort(a,,, {|p, q| p > q})
? a[1]
d := SToD("20240229")
? DoW(d), CDoW(d), CMonth(d), Year(d), Month(d), Day(d), d + 1, d - SToD("20240101")
SET CENTURY ON
? d
dd := {|| x + y}
x := 100
? Eval(dd)
k := 0
DO WHILE .T.
  k++
  IF k == 2
     LOOP
  ENDIF
  IF k > 4
     EXIT
  ENDIF
ENDDO
? "k", k
FOR i := 10 TO 1 STEP -3
   ?? i
NEXT
n := 5
n += 2
n *= 3
? n, n++, n, --n
s := "x"
s += "y"
? s

FUNCTION MakeAdder(n)
RETURN {|v| v + n}

FUNCTION Counter()
   STATIC nCalls := 0
   nCalls++
RETURN nCalls

PROCEDURE Swap(a, b)
   LOCAL t := a
   a := b
   b := t
RETURN

FUNCTION Args()
RETURN PCount()

PROCEDURE ShowPriv
   ? "inner sees", pv
   PRIVATE pv := "inner"
   ? "inner own", pv
RETURN

PROCEDURE SetPub
   gPub := gPub + " changed"
RETURN

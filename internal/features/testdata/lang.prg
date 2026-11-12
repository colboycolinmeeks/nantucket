* simple test
LOCAL i, s := 0
? "Hello, world"
FOR i := 1 TO 10
   s += i
NEXT
? "Sum:", s
x = 5
? x * 2, x / 3, 10 / 4
IF x > 3
   ? "big"
ELSE
   ? "small"
ENDIF
DO CASE
CASE x == 1
   ? "one"
OTHERWISE
   ? "other"
ENDCASE
a := {1, 2, 3}
AAdd(a, 4)
? Len(a), a[4]
b := {|n| n * n}
? Eval(b, 7)
? Upper("abc") + Lower("DEF"), SubStr("hello", 2, 3), Str(3.14159, 8, 2)
? Date() > CToD("01/01/90"), DToS(SToD("20240315")), DToC(SToD("20240315"))
? Fact(10)
? Transform(1234.5, "@E 9,999.99"), Transform("abc", "@!"), Transform(12345.678, "99,999.99")
DO greet WITH "Bob"
n := 0
DO WHILE n < 3
  n++
ENDDO
? "n=", n
FOR EACH v IN {"a","b"}
  ?? v
NEXT
BEGIN SEQUENCE
  Break("oops")
RECOVER USING e
  ? "recovered", e
END
TRY
  ? undefinedvar
CATCH e
  ? "caught:", e:description
END
h := {"a" => 1, "b" => 2}
? h["b"], Len(h)
SWITCH x
CASE 5
  ? "five"
  EXIT
OTHERWISE
  ? "not five"
END
cVar := "x"
? &cVar + 1
RETURN

FUNCTION Fact(n)
   IF n <= 1
      RETURN 1
   ENDIF
RETURN n * Fact(n - 1)

PROCEDURE greet
PARAMETERS who
? "Hello", who
RETURN

* strings_test.prg - example tests using assertions.
* Run with:  nantucket test examples/tests
*
* Code before the first PROCEDURE runs once, before the tests.
PUBLIC cName := "Nantucket"

PROCEDURE TestUpperLower
   AssertEquals("NANTUCKET", Upper(cName))
   AssertEquals("nantucket", Lower(cName))
RETURN

PROCEDURE TestPadding
   AssertEquals("**hi**", PadC("hi", 6, "*"))
   AssertEquals("00042", StrZero(42, 5))
   ASSERT Len(Space(10)) == 10 MESSAGE "Space(10) should be 10 long"
RETURN

PROCEDURE TestErrors
   LOCAL oErr := AssertError({|| "a" + 1})
   AssertEquals("Argument error", oErr:description)
RETURN

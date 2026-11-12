* table_test.prg - an example test checked against expected output.
* The output is compared with table_test.out. After a deliberate change,
* refresh it with:  nantucket test -update examples/tests/table_test.prg
dbCreate("tmp_items", {{"NAME", "C", 10, 0}, {"QTY", "N", 4, 0}})
USE tmp_items
APPEND BLANK
REPLACE name WITH "Apples", qty WITH 12
APPEND BLANK
REPLACE name WITH "Pears", qty WITH 5
LIST
SUM qty TO nTotal
? "Total:", nTotal
ASSERT nTotal == 17
USE
FErase("tmp_items.dbf")

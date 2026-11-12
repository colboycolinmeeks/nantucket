SET DATE BRITISH
dbCreate("cust", {{"NAME","C",15,0},{"CITY","C",10,0},{"AGE","N",3,0},{"BAL","N",9,2},{"JOINED","D",8,0},{"ACTIVE","L",1,0},{"NOTES","M",10,0}})
USE cust
APPEND BLANK
REPLACE name WITH "Smith", city WITH "London", age WITH 42, bal WITH 1234.5, joined WITH CToD("01/02/2020"), active WITH .T.
APPEND BLANK
REPLACE name WITH "Jones", city WITH "Paris", age WITH 31, bal WITH 99.95, active WITH .F., notes WITH "A long memo text"
APPEND BLANK
REPLACE name WITH "Adams", city WITH "London", age WITH 57, bal WITH 0
APPEND BLANK
REPLACE name WITH "Brown", city WITH "Berlin", age WITH 25, bal WITH 500
? RecCount(), FCount()
LIST
LIST name, age FOR city = "London"
GO 2
? name, notes
DISPLAY
INDEX ON name TO custname
LIST name
SEEK "Jo"
? Found(), RecNo(), name
SEEK "Zz"
? Found(), Eof()
INDEX ON age TO custage
LIST name, age
SET INDEX TO custname, custage
SET ORDER TO 2
GO TOP
? name
SET ORDER TO 1
COUNT TO n FOR age > 30
? "count", n
SUM bal, age TO sb, sa
? "sums", sb, sa
AVERAGE age TO avgAge
? "avg", avgAge
LOCATE FOR city = "Berlin"
? Found(), name
GO TOP
DELETE FOR age < 30
SET DELETED ON
LIST name
SET DELETED OFF
PACK
? RecCount()
REPLACE ALL bal WITH bal * 2
LIST name, bal
COPY TO cust2 FOR city = "London"
USE cust2
LIST
USE cust INDEX custname
SET FILTER TO age > 40
LIST name, age
SET FILTER TO
CLOSE ALL
USE cust
? Alias(), Select()
SELECT 2
USE cust2 ALIAS c2
? Alias(), c2->name
SELECT cust
? Alias(), cust->name
CLOSE DATABASES

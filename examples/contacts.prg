* contacts.prg - a small dBase/Clipper style address book
* Run with:  nantucket examples/contacts
#include "inkey.ch"

LOCAL nChoice := 1

SET DATE BRITISH
SET CENTURY ON
SET DELETED ON
SET CONFIRM ON
SET MESSAGE TO 23 CENTER

OpenData()

DO WHILE .T.
   DrawFrame("Contacts")
   @  6, 30 PROMPT " Add a contact      " MESSAGE "Enter a new contact"
   @  7, 30 PROMPT " Browse contacts    " MESSAGE "Scroll and edit the table (Esc to return)"
   @  8, 30 PROMPT " Find by name       " MESSAGE "Look up a contact by name"
   @  9, 30 PROMPT " Report             " MESSAGE "List all contacts"
   @ 10, 30 PROMPT " Quit               " MESSAGE "Leave the program"
   MENU TO nChoice
   DO CASE
   CASE nChoice == 1
      AddContact()
   CASE nChoice == 2
      CLEAR
      BROWSE
   CASE nChoice == 3
      FindContact()
   CASE nChoice == 4
      Report()
   OTHERWISE
      EXIT
   ENDCASE
ENDDO
CLOSE DATABASES
CLEAR
QUIT

STATIC PROCEDURE OpenData()
   IF !File("contacts.dbf")
      dbCreate("contacts", { {"NAME",  "C", 25, 0}, ;
                             {"PHONE", "C", 15, 0}, ;
                             {"CITY",  "C", 15, 0}, ;
                             {"BORN",  "D",  8, 0}, ;
                             {"NOTES", "M", 10, 0} })
   ENDIF
   USE contacts
   IF !File("contname.ntx")
      INDEX ON Upper(name) TO contname
   ELSE
      SET INDEX TO contname
   ENDIF
RETURN

STATIC PROCEDURE DrawFrame(cTitle)
   CLEAR
   @ 0, 0 TO 24, 79 DOUBLE
   @ 1, (80 - Len(cTitle)) / 2 SAY cTitle
   @ 2, 1 TO 2, 78
   @ 22, 2 SAY "Records: " + LTrim(Str(RecCount())) + "   " + DToC(Date())
RETURN

STATIC PROCEDURE AddContact()
   LOCAL cName := Space(25), cPhone := Space(15), cCity := Space(15), dBorn := CToD("")
   DrawFrame("Add a contact")
   @  6, 10 SAY "Name  " GET cName  PICTURE "@!" VALID !Empty(cName)
   @  8, 10 SAY "Phone " GET cPhone PICTURE "@R (999) 999-9999"
   @ 10, 10 SAY "City  " GET cCity
   @ 12, 10 SAY "Born  " GET dBorn
   READ
   IF LastKey() != K_ESC .AND. !Empty(cName)
      APPEND BLANK
      REPLACE name WITH cName, phone WITH cPhone, city WITH cCity, born WITH dBorn
      Alert("Saved " + Trim(cName))
   ENDIF
RETURN

STATIC PROCEDURE FindContact()
   LOCAL cName := Space(25)
   DrawFrame("Find a contact")
   @ 6, 10 SAY "Name starts with " GET cName PICTURE "@!"
   READ
   IF Empty(cName)
      RETURN
   ENDIF
   SEEK Trim(cName)
   IF Found()
      @  9, 10 SAY "Name : " + name
      @ 10, 10 SAY "Phone: " + Transform(phone, "@R (999) 999-9999")
      @ 11, 10 SAY "City : " + city
      @ 12, 10 SAY "Born : " + DToC(born)
   ELSE
      @ 9, 10 SAY "Not found."
   ENDIF
   @ 20, 10 SAY "Press any key..."
   Inkey(0)
RETURN

STATIC PROCEDURE Report()
   LOCAL nRow := 4
   DrawFrame("All contacts")
   GO TOP
   DO WHILE !Eof() .AND. nRow < 21
      @ nRow, 3 SAY name + " " + city + " " + DToC(born)
      nRow++
      SKIP
   ENDDO
   @ 21, 3 SAY "Press any key..."
   Inkey(0)
RETURN

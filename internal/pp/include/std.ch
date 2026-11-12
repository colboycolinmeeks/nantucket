/*
 * std.ch - standard xBase command definitions.
 * Later rules take precedence over earlier ones.
 */
#include "set.ch"

/* ---------------- console output ---------------- */
#command ? [<list,...>]   => QOut( <list> )
#command ?? [<list,...>]  => QQOut( <list> )
#command EJECT            => __Eject()

/* ---------------- screen ---------------- */
#command CLS                    => Scroll() ; SetPos( 0, 0 )
#command CLEAR SCREEN           => CLS
#command CLEAR GETS             => GetList := {}
#command CLEAR                  => CLEAR SCREEN ; CLEAR GETS
#command CLEAR MEMORY           => __MClear()
#command CLEAR TYPEAHEAD        => __Keyboard()
#command CLEAR ALL              => dbCloseAll() ; __MClear() ; GetList := {} ; Set( _SET_ALTFILE, "" )

#command @ <row>, <col>                         => Scroll( <row>, <col>, <row> ) ; SetPos( <row>, <col> )
#command @ <row>, <col> CLEAR                   => Scroll( <row>, <col> ) ; SetPos( <row>, <col> )
#command @ <top>, <left> CLEAR TO <bottom>, <right> => Scroll( <top>, <left>, <bottom>, <right> ) ; SetPos( <top>, <left> )
#command @ <top>, <left>, <bottom>, <right> BOX <string> [COLOR <color>] => DispBox( <top>, <left>, <bottom>, <right>, <string> [, <color>] )
#command @ <top>, <left> TO <bottom>, <right> [COLOR <color>]        => DispBox( <top>, <left>, <bottom>, <right>, 1 [, <color>] )
#command @ <top>, <left> TO <bottom>, <right> DOUBLE [COLOR <color>] => DispBox( <top>, <left>, <bottom>, <right>, 2 [, <color>] )

#command @ <row>, <col> SAY <xpr> [COLOR <color>] => DevPos( <row>, <col> ) ; DevOut( <xpr> [, <color>] )
#command @ <row>, <col> SAY <xpr> [PICTURE <pic>] [COLOR <color>] => DevPos( <row>, <col> ) ; DevOutPict( <xpr>, <pic> [, <color>] )

#command @ <row>, <col> GET <var> [PICTURE <pic>] [VALID <valid>] [WHEN <when>] [RANGE <lo>, <hi>] [COLOR <color>] => ;
         SetPos( <row>, <col> ) ; AAdd( GetList, __GetNew( {|_1| iif( _1 == NIL, <var>, <var> := _1 )}, <"var">, <pic>, <{valid}>, <{when}>, <lo>, <hi>, <color> ) )

#command @ <row>, <col> SAY <sayxpr> [PICTURE <saypic>] [COLOR <saycolor>] GET <var> [PICTURE <pic>] [VALID <valid>] [WHEN <when>] [RANGE <lo>, <hi>] [COLOR <color>] => ;
         @ <row>, <col> SAY <sayxpr> [PICTURE <saypic>] [COLOR <saycolor>] ; ;
         @ Row(), Col() + 1 GET <var> [PICTURE <pic>] [VALID <valid>] [WHEN <when>] [RANGE <lo>, <hi>] [COLOR <color>]

#command @ <row>, <col> PROMPT <prompt> [MESSAGE <msg>] => __AtPrompt( <row>, <col>, <prompt>, <msg> )
#command MENU TO <v> => <v> := __MenuTo( {|_1| iif( _1 == NIL, <v>, <v> := _1 )}, <"v"> )

#command READ SAVE => ReadModal( GetList )
#command READ      => ReadModal( GetList ) ; GetList := {}

/* ---------------- windows ---------------- */
#command SET WINDOW TO <n>       => WSelect( <n> )
#command SET WINDOW TO           => WSelect( 0 )
#command WINDOW OPEN <t>, <l> TO <b>, <r> [TITLE <title>] [COLOR <color>] [<fix: FIXED>] => ;
         WOpen( <t>, <l>, <b>, <r>, <title>, <color>, !<.fix.> )
#command WINDOW OPEN <t>, <l> TO <b>, <r> [TITLE <title>] [COLOR <color>] [<fix: FIXED>] TO <var> => ;
         <var> := WOpen( <t>, <l>, <b>, <r>, <title>, <color>, !<.fix.> )
#command WINDOW CLOSE [<n>]      => WClose( <n> )
#command WINDOW CLOSE ALL        => WAClose()
#command WINDOW MOVE <n> TO <t>, <l>       => WMove( <t>, <l>, <n> )
#command WINDOW SIZE <n> TO <rows>, <cols> => WSize( <rows>, <cols>, <n> )
#command WINDOW MINIMIZE [<n>]   => WMinimize( <n> )
#command WINDOW MAXIMIZE [<n>]   => WMaximize( <n> )
#command WINDOW RESTORE [<n>]    => WRestore( <n> )

/* ---------------- interaction ---------------- */
#command WAIT [<c>]              => __Wait( <c> )
#command WAIT [<c>] TO <var>     => <var> := __Wait( <c> )
#command ACCEPT [<c>] TO <var>   => <var> := __Accept( <c> )
#command INPUT [<c>] TO <var>    => <var> := __Input( <c> )
#command KEYBOARD <c>            => __Keyboard( <c> )
#command QUIT                    => __Quit()
#command CANCEL                  => __Cancel()
#command RETURN TO MASTER        => __Cancel()
#command RUN <*cmd*>             => __Run( #<cmd> )
#command ! <*cmd*>               => __Run( #<cmd> )
#command HELP                    => __Help()
#command CD                      => __ChDir( "" )
#command CD <*dir*>              => __ChDir( #<dir> )
#command CHDIR                   => __ChDir( "" )
#command CHDIR <*dir*>           => __ChDir( #<dir> )
#command VERSION                 => __Version( .F. )
#command VERSION FULL            => __Version( .T. )

/* ---------------- testing ---------------- */
#command ASSERT <cond> [MESSAGE <msg>] => __Assert( <cond>, <"cond">, <msg> )

/* ---------------- memory variables ---------------- */
#command STORE <value> TO <var1> [, <varN>]  => <var1> := [ <varN> := ] <value>
#command RELEASE <vars,...>                  => __MXRelease( <"vars"> )
#command RELEASE ALL                         => __MRelease( "*", .T. )
#command RELEASE ALL LIKE <(skel)>           => __MRelease( <(skel)>, .T. )
#command RELEASE ALL EXCEPT <(skel)>         => __MRelease( <(skel)>, .F. )
#command SAVE TO <(file)> [ALL]              => __MSave( <(file)>, "*", .T. )
#command SAVE TO <(file)> ALL LIKE <(skel)>  => __MSave( <(file)>, <(skel)>, .T. )
#command SAVE TO <(file)> ALL EXCEPT <(skel)> => __MSave( <(file)>, <(skel)>, .F. )
#command SAVE ALL LIKE <(skel)> TO <(file)>  => __MSave( <(file)>, <(skel)>, .T. )
#command RESTORE FROM <(file)> [<add: ADDITIVE>] => __MRestore( <(file)>, <.add.> )
#command DISPLAY MEMORY                      => __MDisp()
#command LIST MEMORY                         => __MDisp()
#xcommand DEFAULT <v1> TO <x1> [, <vn> TO <xn>] => ;
          iif( <v1> == NIL, <v1> := <x1>, NIL ) [; iif( <vn> == NIL, <vn> := <xn>, NIL )]

/* ---------------- SET commands ---------------- */
#command SET EXACT <x:ON,OFF,&>       => Set( _SET_EXACT, <(x)> )
#command SET TRUSTED <x:ON,OFF,&>     => __SetTrusted( <(x)> )
#command SET CODEPAGE TO <(x)>        => __SetCodePage( <(x)> )
#command SET CODEPAGE TO              => __SetCodePage( "" )
#command SET EXACT (<x>)              => Set( _SET_EXACT, <x> )
#command SET FIXED <x:ON,OFF,&>       => Set( _SET_FIXED, <(x)> )
#command SET FIXED (<x>)              => Set( _SET_FIXED, <x> )
#command SET EXCLUSIVE <x:ON,OFF,&>   => Set( _SET_EXCLUSIVE, <(x)> )
#command SET EXCLUSIVE (<x>)          => Set( _SET_EXCLUSIVE, <x> )
#command SET SOFTSEEK <x:ON,OFF,&>    => Set( _SET_SOFTSEEK, <(x)> )
#command SET SOFTSEEK (<x>)           => Set( _SET_SOFTSEEK, <x> )
#command SET UNIQUE <x:ON,OFF,&>      => Set( _SET_UNIQUE, <(x)> )
#command SET UNIQUE (<x>)             => Set( _SET_UNIQUE, <x> )
#command SET DELETED <x:ON,OFF,&>     => Set( _SET_DELETED, <(x)> )
#command SET DELETED (<x>)            => Set( _SET_DELETED, <x> )
#command SET CONFIRM <x:ON,OFF,&>     => Set( _SET_CONFIRM, <(x)> )
#command SET CONFIRM (<x>)            => Set( _SET_CONFIRM, <x> )
#command SET ESCAPE <x:ON,OFF,&>      => Set( _SET_ESCAPE, <(x)> )
#command SET ESCAPE (<x>)             => Set( _SET_ESCAPE, <x> )
#command SET INTENSITY <x:ON,OFF,&>   => Set( _SET_INTENSITY, <(x)> )
#command SET SCOREBOARD <x:ON,OFF,&>  => Set( _SET_SCOREBOARD, <(x)> )
#command SET WRAP <x:ON,OFF,&>        => Set( _SET_WRAP, <(x)> )
#command SET WRAP (<x>)               => Set( _SET_WRAP, <x> )
#command SET BELL <x:ON,OFF,&>        => Set( _SET_BELL, <(x)> )
#command SET CONSOLE <x:ON,OFF,&>     => Set( _SET_CONSOLE, <(x)> )
#command SET CONSOLE (<x>)            => Set( _SET_CONSOLE, <x> )
#command SET ALTERNATE <x:ON,OFF,&>   => Set( _SET_ALTERNATE, <(x)> )
#command SET ALTERNATE (<x>)          => Set( _SET_ALTERNATE, <x> )
#command SET PRINTER <x:ON,OFF,&>     => Set( _SET_PRINTER, <(x)> )
#command SET PRINT <x:ON,OFF,&>       => Set( _SET_PRINTER, <(x)> )
#command SET TALK <x:ON,OFF,&>        => Set( _SET_TALK, <(x)> )
#command SET SAFETY <x:ON,OFF,&>      => Set( _SET_SAFETY, <(x)> )
#command SET HEADING <x:ON,OFF,&>     => Set( _SET_HEADING, <(x)> )
#command SET STATUS <x:ON,OFF,&>      => Set( _SET_STATUS, <(x)> )
#command SET ECHO <x:ON,OFF,&>        => Set( _SET_ECHO, <(x)> )
#command SET INSERT <x:ON,OFF,&>      => Set( _SET_INSERT, <(x)> )
#command SET DELIMITERS <x:ON,OFF,&>  => Set( _SET_DELIMITERS, <(x)> )
#command SET DELIMITERS TO <c>        => Set( _SET_DELIMCHARS, <c> )
#command SET DELIMITERS TO DEFAULT    => Set( _SET_DELIMCHARS, "::" )
#command SET CENTURY <x:ON,OFF,&>     => __SetCentury( <(x)> )
#command SET CENTURY (<x>)            => __SetCentury( <x> )
#command SET CURSOR <x:ON,OFF,&>      => SetCursor( iif( Upper( <(x)> ) == "ON", 1, 0 ) )
#command SET CURSOR (<x>)             => SetCursor( iif( <x>, 1, 0 ) )
#command SET STEP <x:ON,OFF,&>        => __SetStep( <(x)> )
#command SET DEBUG <x:ON,OFF,&>       => NIL
#command SET DOHISTORY <x:ON,OFF,&>   => NIL
#command SET MENU <x:ON,OFF,&>        => NIL

#command SET DATE [TO] AMERICAN   => _DFSET( "mm/dd/yyyy", "mm/dd/yy" )
#command SET DATE [TO] ANSI       => _DFSET( "yyyy.mm.dd", "yy.mm.dd" )
#command SET DATE [TO] BRITISH    => _DFSET( "dd/mm/yyyy", "dd/mm/yy" )
#command SET DATE [TO] FRENCH     => _DFSET( "dd/mm/yyyy", "dd/mm/yy" )
#command SET DATE [TO] GERMAN     => _DFSET( "dd.mm.yyyy", "dd.mm.yy" )
#command SET DATE [TO] ITALIAN    => _DFSET( "dd-mm-yyyy", "dd-mm-yy" )
#command SET DATE [TO] JAPANESE   => _DFSET( "yyyy/mm/dd", "yy/mm/dd" )
#command SET DATE [TO] USA        => _DFSET( "mm-dd-yyyy", "mm-dd-yy" )
#command SET DATE [TO] YMD        => _DFSET( "yyyy/mm/dd", "yy/mm/dd" )
#command SET DATE [TO] DMY        => _DFSET( "dd/mm/yyyy", "dd/mm/yy" )
#command SET DATE [TO] MDY        => _DFSET( "mm/dd/yyyy", "mm/dd/yy" )
#command SET DATE FORMAT [TO] <c> => Set( _SET_DATEFORMAT, <c> )
#command SET EPOCH TO <n>         => Set( _SET_EPOCH, <n> )
#command SET DECIMALS TO <n>      => Set( _SET_DECIMALS, <n> )
#command SET DECIMALS TO          => Set( _SET_DECIMALS, 0 )
#command SET MARGIN TO <n>        => Set( _SET_MARGIN, <n> )
#command SET MARGIN TO            => Set( _SET_MARGIN, 0 )
#command SET TYPEAHEAD TO <n>     => Set( _SET_TYPEAHEAD, <n> )
#command SET MEMOWIDTH TO <n>     => Set( _SET_MEMOWIDTH, <n> )
#command SET MESSAGE TO <n> [<cent: CENTER, CENTRE>] => Set( _SET_MESSAGE, <n> ) ; Set( _SET_MCENTER, <.cent.> )
#command SET MESSAGE TO           => Set( _SET_MESSAGE, 0 ) ; Set( _SET_MCENTER, .F. )
#command SET PATH TO <*path*>     => Set( _SET_PATH, #<path> )
#command SET PATH TO              => Set( _SET_PATH, "" )
#command SET DEFAULT TO <(path)>  => Set( _SET_DEFAULT, <(path)> )
#command SET DEFAULT TO           => Set( _SET_DEFAULT, "" )
#command SET COLOR TO [<*spec*>]  => SetColor( #<spec> )
#command SET COLOR TO ( <c> )     => SetColor( <c> )
#command SET COLOUR TO [<*spec*>] => SetColor( #<spec> )
#command SET THEME TO <(name)>    => SetTheme( <(name)> )
#command SET THEME TO             => SetTheme( "" )
#command SET ALTERNATE TO <(file)> [<add: ADDITIVE>] => Set( _SET_ALTFILE, <(file)>, <.add.> )
#command SET ALTERNATE TO         => Set( _SET_ALTFILE, "" )
#command SET PRINTER TO <(file)> [<add: ADDITIVE>]   => Set( _SET_PRINTFILE, <(file)>, <.add.> )
#command SET PRINTER TO           => Set( _SET_PRINTFILE, "" )
#command SET DEVICE TO SCREEN     => Set( _SET_DEVICE, "SCREEN" )
#command SET DEVICE TO PRINTER    => Set( _SET_DEVICE, "PRINTER" )
#command SET KEY <n> TO <proc>    => SetKey( <n>, {|p, l, v| <proc>( p, l, v )} )
#command SET KEY <n> TO           => SetKey( <n>, NIL )
#command SET FUNCTION <n> [TO] <c> => __SetFunction( <n>, <c> )
#command SET PROCEDURE TO <(f)> [<add: ADDITIVE>] => __SetProc( <(f)>, <.add.> )
#command SET PROCEDURE TO         => __SetProc( "" )
#command SET FORMAT TO <*x*>      => NIL
#command SET FORMAT TO            => NIL

/* ---------------- databases ---------------- */
#command USE                       => dbCloseArea()
#command USE <(db)> [VIA <rdd>] [ALIAS <a>] [<new: NEW>] [<ex: EXCLUSIVE>] [<sh: SHARED>] [<ro: READONLY>] [INDEX <(index1)> [, <(indexn)>]] => ;
         dbUseArea( <.new.>, <rdd>, <(db)>, <(a)>, iif( <.sh.> .or. <.ex.>, !<.ex.>, NIL ), <.ro.> ) ;
         [; dbSetIndex( <(index1)> )] [; dbSetIndex( <(indexn)> )]

#command SELECT <whatever>         => dbSelectArea( <(whatever)> )
#command SELECT <f>([<list,...>])  => dbSelectArea( <f>( <list> ) )

#command CLOSE                     => dbCloseArea()
#command CLOSE <alias>             => <alias>->( dbCloseArea() )
#command CLOSE DATABASES           => dbCloseAll()
#command CLOSE ALTERNATE           => Set( _SET_ALTFILE, "" )
#command CLOSE FORMAT              => NIL
#command CLOSE INDEXES             => dbClearIndex()
#command CLOSE PROCEDURE           => __SetProc( "" )
#command CLOSE ALL                 => dbCloseAll() ; Set( _SET_ALTFILE, "" ) ; Set( _SET_PRINTFILE, "" )

#command APPEND BLANK              => dbAppend()
#command APPEND                    => __dbAppendEdit()
#command APPEND FROM <(file)> [FIELDS <fields,...>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] [VIA <rdd>] [<sdf: SDF>] [<del: DELIMITED>] [WITH <*delim*>] => ;
         __dbApp( <(file)>, { <(fields)> }, <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.sdf.>, <.del.>, #<delim> )

#command COPY TO <(file)> [FIELDS <fields,...>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] [VIA <rdd>] [<sdf: SDF>] [<del: DELIMITED>] [WITH <*delim*>] => ;
         __dbCopy( <(file)>, { <(fields)> }, <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.sdf.>, <.del.>, #<delim> )
#command COPY STRUCTURE TO <(file)> [FIELDS <fields,...>] => __dbCopyStruct( <(file)>, { <(fields)> } )
#command COPY STRUCTURE EXTENDED TO <(file)>             => __dbCopyXStruct( <(file)> )
#command COPY FILE <(src)> TO <(dest)>                    => __CopyFile( <(src)>, <(dest)> )

#command CREATE <(file)>                                  => __dbCreateInteractive( <(file)> )
#command CREATE <(file)> FROM <(xfile)> [<new: NEW>] [ALIAS <a>] => __dbCreate( <(file)>, <(xfile)>, NIL, <.new.>, <(a)> )
#command MODIFY STRUCTURE                                 => __dbModStru()
#command CREATE REPORT                                    => __ReportDesign( "", .T. )
#command CREATE REPORT <(file)>                           => __ReportDesign( <(file)>, .T. )
#command CREATE REPORT <(file)> FROM <fields,...>         => __ReportCreate( <(file)>, { <(fields)> } )
#command MODIFY REPORT                                    => __ReportDesign( "", .F. )
#command MODIFY REPORT <(file)>                           => __ReportDesign( <(file)>, .F. )
#command CREATE LABEL                                     => __LabelDesign( "", .T. )
#command CREATE LABEL <(file)>                            => __LabelDesign( <(file)>, .T. )
#command CREATE LABEL <(file)> FROM <fields,...>          => __LabelCreate( <(file)>, { <(fields)> } )
#command MODIFY LABEL                                     => __LabelDesign( "", .F. )
#command MODIFY LABEL <(file)>                            => __LabelDesign( <(file)>, .F. )

#command PACK                      => __dbPack()
#command ZAP                       => __dbZap()
#command UNLOCK                    => dbUnlock()
#command UNLOCK ALL                => dbUnlockAll()
#command COMMIT                    => dbCommitAll()

#command GOTO <n>                  => dbGoto( <n> )
#command GO <n>                    => dbGoto( <n> )
#command GOTO TOP                  => dbGoTop()
#command GO TOP                    => dbGoTop()
#command GOTO BOTTOM               => dbGoBottom()
#command GO BOTTOM                 => dbGoBottom()

#command SKIP                      => dbSkip( 1 )
#command SKIP <n>                  => dbSkip( <n> )
#command SKIP ALIAS <a>            => <a>->( dbSkip( 1 ) )
#command SKIP <n> ALIAS <a>        => <a>->( dbSkip( <n> ) )

#command SEEK <xpr> [<soft: SOFTSEEK>] [<last: LAST>] => dbSeek( <xpr>, iif( <.soft.>, .T., NIL ), iif( <.last.>, .T., NIL ) )
#command FIND <*text*>             => __dbFind( #<text> )
#command CONTINUE                  => __dbContinue()
#command LOCATE [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbLocate( <{for}>, <{while}>, <next>, <rec>, <.rest.> )

#command DELETE [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         DBEval( {|| dbDelete()}, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command DELETE                    => dbDelete()
#command RECALL [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         DBEval( {|| dbRecall()}, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command RECALL                    => dbRecall()
#command DELETE FILE <(file)>      => FErase( <(file)> )
#command ERASE <(file)>            => FErase( <(file)> )
#command RENAME <(old)> TO <(new)> => FRename( <(old)>, <(new)> )

#command REPLACE [<f1> WITH <x1> [, <fn> WITH <xn>]] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         DBEval( {|| _FIELD-><f1> := <x1> [, _FIELD-><fn> := <xn>]}, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command REPLACE <f1> WITH <x1> [, <fn> WITH <xn>] => _FIELD-><f1> := <x1> [; _FIELD-><fn> := <xn>]

#command COUNT [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbCount( NIL, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command COUNT TO <var> [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbCount( {|_1| <var> := _1}, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command SUM [<x1> [, <xn>]] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbSum( .F., { <{x1}> [, <{xn}>] }, { <"x1"> [, <"xn">] }, NIL, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command SUM <x1> [, <xn>] TO <v1> [, <vn>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbSum( .F., { <{x1}> [, <{xn}>] }, { <"x1"> [, <"xn">] }, { {|_1| <v1> := _1} [, {|_1| <vn> := _1}] }, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command AVERAGE [<x1> [, <xn>]] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbSum( .T., { <{x1}> [, <{xn}>] }, { <"x1"> [, <"xn">] }, NIL, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command AVERAGE <x1> [, <xn>] TO <v1> [, <vn>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbSum( .T., { <{x1}> [, <{xn}>] }, { <"x1"> [, <"xn">] }, { {|_1| <v1> := _1} [, {|_1| <vn> := _1}] }, <{for}>, <{while}>, <next>, <rec>, <.rest.> )

#command LIST [<list,...>] [<off:OFF>] [<toPrint: TO PRINTER>] [TO FILE <(toFile)>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbList( <.off.>, { <{list}> }, { <"list"> }, .T., <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.toPrint.>, <(toFile)> )
#command DISPLAY [<list,...>] [<off:OFF>] [<toPrint: TO PRINTER>] [TO FILE <(toFile)>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [<all:ALL>] => ;
         __dbList( <.off.>, { <{list}> }, { <"list"> }, <.all.>, <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.toPrint.>, <(toFile)> )
#command DISPLAY STRUCTURE [TO PRINTER] => __dbDispStru( .F. )
#command LIST STRUCTURE [TO PRINTER]    => __dbDispStru( .T. )
#command DISPLAY STATUS                 => __DispStatus()
#command LIST STATUS                    => __DispStatus()
#command REPORT FORM <(frm)> [HEADING <h>] [<noej: NOEJECT>] [<plain: PLAIN>] [<sum: SUMMARY>] [<nocons: NOCONSOLE>] [<toPrint: TO PRINTER>] [TO FILE <(toFile)>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __ReportForm( <(frm)>, <.toPrint.>, <(toFile)>, <.nocons.>, <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.plain.>, <h>, <.noej.>, <.sum.> )
#command LABEL FORM <(lbl)> [<sample: SAMPLE>] [<nocons: NOCONSOLE>] [<toPrint: TO PRINTER>] [TO FILE <(toFile)>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __LabelForm( <(lbl)>, <.toPrint.>, <(toFile)>, <.nocons.>, <{for}>, <{while}>, <next>, <rec>, <.rest.>, <.sample.> )

#command INDEX ON <key> TO <(file)> [FOR <for>] [<u: UNIQUE>] [<d: DESCENDING>] => ;
         dbCreateIndex( <(file)>, <"key">, <{key}>, iif( <.u.>, .T., NIL ), <"for">, <{for}>, <.d.> )
#command REINDEX                   => dbReindex()
#command SET INDEX TO [<(index1)> [, <(indexn)>]] [<add: ADDITIVE>] => ;
         iif( <.add.>, NIL, dbClearIndex() ) [; dbSetIndex( <(index1)> )] [; dbSetIndex( <(indexn)> )]
#command SET ORDER TO <n>          => dbSetOrder( <n> )
#command SET ORDER TO              => dbSetOrder( 0 )

#command SET FILTER TO             => dbClearFilter()
#command SET FILTER TO <xpr>       => dbSetFilter( <{xpr}>, <"xpr"> )
#command SET FILTER TO <x:&>       => iif( Empty( <(x)> ), dbClearFilter(), dbSetFilter( <{x}>, <(x)> ) )

#command SET RELATION TO           => dbClearRelation()
#command SET RELATION [<add: ADDITIVE>] TO <key1> INTO <(alias1)> [, [TO] <keyn> INTO <(aliasn)>] => ;
         iif( <.add.>, NIL, dbClearRelation() ) ; dbSetRelation( <(alias1)>, <{key1}>, <"key1"> ) [; dbSetRelation( <(aliasn)>, <{keyn}>, <"keyn"> )]

#command SORT TO <(file)> ON <fields,...> [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbSort( <(file)>, { <(fields)> }, <{for}>, <{while}>, <next>, <rec>, <.rest.> )
#command TOTAL ON <key> TO <(file)> [FIELDS <fields,...>] [FOR <for>] [WHILE <while>] [NEXT <next>] [RECORD <rec>] [<rest:REST>] [ALL] => ;
         __dbTotal( <(file)>, <{key}>, { <(fields)> }, <{for}>, <{while}>, <next>, <rec>, <.rest.> )

#command BROWSE [FIELDS <f,...>]   => Browse( , , , , { <(f)> } )
#command EDIT                      => __dbEdit()
#command CHANGE                    => __dbEdit()

/* ---------------- files and programs ---------------- */
#command MODIFY COMMAND <(file)>   => __ModiComm( <(file)> )
#command MODIFY COMMAND            => __ModiComm( "" )
#command MODIFY FILE <(file)>      => __ModiFile( <(file)> )
#command DIR [<*spec*>]            => __Dir( #<spec> )
#command DIRECTORY [<*spec*>]      => __Dir( #<spec> )
#command TYPE <(file)> [TO PRINTER] => __TypeFile( <(file)> )
#command COMPILE <(file)>          => __Compile( <(file)> )

/* rules that must win over the general LIST/DISPLAY/SET COLOR rules */
#command DISPLAY MEMORY            => __MDisp()
#command LIST MEMORY               => __MDisp()
#command SET COLOR TO              => SetColor( "" )
#command SET COLOUR TO             => SetColor( "" )

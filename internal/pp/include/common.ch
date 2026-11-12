#define TRUE  .T.
#define FALSE .F.
#define YES   .T.
#define NO    .F.
#translate ISNIL( <v1> )       => ( <v1> == NIL )
#translate ISARRAY( <v1> )     => ( ValType( <v1> ) == "A" )
#translate ISBLOCK( <v1> )     => ( ValType( <v1> ) == "B" )
#translate ISCHARACTER( <v1> ) => ( ValType( <v1> ) == "C" )
#translate ISDATE( <v1> )      => ( ValType( <v1> ) == "D" )
#translate ISLOGICAL( <v1> )   => ( ValType( <v1> ) == "L" )
#translate ISMEMO( <v1> )      => ( ValType( <v1> ) == "M" )
#translate ISNUMBER( <v1> )    => ( ValType( <v1> ) == "N" )
#translate ISOBJECT( <v1> )    => ( ValType( <v1> ) == "O" )
#xcommand UPDATE <v1> IF <exp> TO <v2> => IF <exp> ; <v1> := <v2> ; END

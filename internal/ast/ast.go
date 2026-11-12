// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package ast defines the syntax tree produced by the parser.
package ast

import "nantucket/internal/value"

type Expr interface{ exprNode() }
type Stmt interface{ stmtNode() }

type Pos struct{ Line int }

// ---------- expressions ----------

type (
	Lit struct {
		V value.Value
	}
	Ident struct { // unqualified name, resolved by compiler
		Name string
	}
	FieldRef struct { // FIELD->name, alias->name
		Alias     string // "" = current area
		AliasExpr Expr   // (expr)->name
		Name      string
	}
	MemvarRef struct { // M->name / MEMVAR->name
		Name string
	}
	AliasExpr struct { // alias->( expr )
		Alias     string
		AliasExpr Expr
		X         Expr
	}
	Macro struct { // &name, &name.ext, &( expr )
		Text string // raw text for &name forms
		X    Expr   // for &( expr )
	}
	Call struct {
		Name string // function name (upper)
		Args []Expr
	}
	CallMacro struct { // &name( args )
		Callee Expr
		Args   []Expr
	}
	Index struct {
		X   Expr
		Idx Expr
	}
	Send struct { // obj:msg(args)
		X     Expr
		Msg   string
		Args  []Expr
		IsVar bool // obj:field (no parens)
	}
	Unary struct {
		Op string
		X  Expr
	}
	Binary struct {
		Op   string
		L, R Expr
	}
	Assign struct {
		Op     string // ":=", "+=", ...
		Target Expr
		X      Expr
	}
	IncDec struct {
		Op     string // "++" or "--"
		Prefix bool
		Target Expr
	}
	ArrayLit struct {
		Items []Expr
	}
	HashLit struct {
		Keys, Vals []Expr
	}
	Block struct {
		Params []string
		Body   []Expr
		Src    string
	}
	IIf struct {
		C, T, F Expr
	}
	RefArg struct { // @var
		X Expr
	}
	Empty struct{} // omitted argument
)

func (*Lit) exprNode()       {}
func (*Ident) exprNode()     {}
func (*FieldRef) exprNode()  {}
func (*MemvarRef) exprNode() {}
func (*AliasExpr) exprNode() {}
func (*Macro) exprNode()     {}
func (*Call) exprNode()      {}
func (*CallMacro) exprNode() {}
func (*Index) exprNode()     {}
func (*Send) exprNode()      {}
func (*Unary) exprNode()     {}
func (*Binary) exprNode()    {}
func (*Assign) exprNode()    {}
func (*IncDec) exprNode()    {}
func (*ArrayLit) exprNode()  {}
func (*HashLit) exprNode()   {}
func (*Block) exprNode()     {}
func (*IIf) exprNode()       {}
func (*RefArg) exprNode()    {}
func (*Empty) exprNode()     {}

// ---------- statements ----------

type VarDecl struct {
	Name string
	Dims []Expr // PRIVATE a[10]
	Init Expr
}

type (
	ExprStmt struct {
		Line int
		X    Expr
	}
	LocalStmt struct {
		Line   int
		Static bool
		Vars   []VarDecl
	}
	MemvarStmt struct { // PRIVATE / PUBLIC / DECLARE / DIMENSION
		Line   int
		Public bool
		Vars   []VarDecl
		Macro  []Expr // PRIVATE &x
	}
	ParamStmt struct {
		Line  int
		Names []string
	}
	DeclStmt struct { // MEMVAR / FIELD declarations
		Line  int
		Field bool
		Names []string
		Alias string
	}
	IfStmt struct {
		Line   int
		Conds  []Expr
		Bodies [][]Stmt
		Else   []Stmt
	}
	WhileStmt struct {
		Line int
		Cond Expr
		Body []Stmt
	}
	ForStmt struct {
		Line           int
		Var            Expr
		From, To, Step Expr
		Body           []Stmt
	}
	ForEachStmt struct {
		Line int
		Var  Expr
		In   Expr
		Desc bool
		Body []Stmt
	}
	CaseStmt struct {
		Line   int
		Conds  []Expr
		Bodies [][]Stmt
		Other  []Stmt
	}
	SwitchStmt struct {
		Line   int
		X      Expr
		Vals   []Expr
		Bodies [][]Stmt
		Other  []Stmt
	}
	SeqStmt struct { // BEGIN SEQUENCE / TRY
		Line       int
		Body       []Stmt
		RecoverVar Expr
		Recover    []Stmt
		HasRecover bool
		Always     []Stmt
		Try        bool
		With       Expr // BEGIN SEQUENCE WITH <block>: error handler for the body
	}
	ReturnStmt struct {
		Line int
		X    Expr
	}
	ExitStmt struct{ Line int }
	LoopStmt struct{ Line int }
	DoStmt   struct {
		Line  int
		Name  string
		Macro Expr
		Args  []Expr
	}
	BreakStmt struct {
		Line int
		X    Expr
	}
	NopStmt struct{}
)

func (*ExprStmt) stmtNode()    {}
func (*LocalStmt) stmtNode()   {}
func (*MemvarStmt) stmtNode()  {}
func (*ParamStmt) stmtNode()   {}
func (*DeclStmt) stmtNode()    {}
func (*IfStmt) stmtNode()      {}
func (*WhileStmt) stmtNode()   {}
func (*ForStmt) stmtNode()     {}
func (*ForEachStmt) stmtNode() {}
func (*CaseStmt) stmtNode()    {}
func (*SwitchStmt) stmtNode()  {}
func (*SeqStmt) stmtNode()     {}
func (*ReturnStmt) stmtNode()  {}
func (*ExitStmt) stmtNode()    {}
func (*LoopStmt) stmtNode()    {}
func (*DoStmt) stmtNode()      {}
func (*BreakStmt) stmtNode()   {}
func (*NopStmt) stmtNode()     {}

// Func is a FUNCTION or PROCEDURE.
type Func struct {
	Name     string
	Params   []string
	IsProc   bool
	Static   bool
	Init     bool // INIT PROCEDURE
	Exit     bool
	Body     []Stmt
	Line     int
	Implicit bool // top-level code of a file
}

// File is a parsed compile unit.
type File struct {
	Name    string
	Funcs   []*Func
	Statics []*LocalStmt // file-wide statics
	Fields  []*DeclStmt
	Memvars []*DeclStmt
}

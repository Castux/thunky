package syntax

import (
	"fmt"

	"thunky/internal/source"
)

type Parser struct {
	Tokens   []Token
	Head     int
	PosStack []source.SourcePos
}

func (p *Parser) Is(kind string) bool {
	if p.Head >= len(p.Tokens) {
		return false
	}
	return p.Tokens[p.Head].Kind == kind
}

func (p *Parser) Peek(offset int) Token {
	return p.Tokens[p.Head+offset]
}

func (p *Parser) Pos() source.SourcePos {
	return p.Peek(0).Pos
}

func (p *Parser) PrevPos() source.SourcePos {
	return p.Peek(-1).Pos
}

func (p *Parser) Consume() Token {
	tok := p.Peek(0)
	p.Head++
	return tok
}

func (p *Parser) Accept(kind string) bool {
	tok := p.Peek(0)
	if tok.Kind == kind {
		p.Head++
		return true
	}
	return false
}

func (p *Parser) Expect(kind string) Token {
	tok := p.Peek(0)
	if tok.Kind == kind {
		p.Head++
		return tok
	}
	source.Log(fmt.Sprintf("expected %s, found %s instead", kind, tok.Kind), tok.Pos, source.SeverityError)
	panic("expect")
}

func (p *Parser) ParseProgram() *Program {
	start := p.Pos()

	imports := []*Name{}
	if p.Accept("import") {
		for {
			imports = append(imports, p.ParseName())
			if !p.Accept(",") {
				break
			}
		}
		p.Expect("in")
	}
	expr := p.ParseExpression()
	p.Expect("eof")

	return &Program{Imports: imports, Body: expr, Start: start}
}

func (p *Parser) ParseModule() *Module {
	start := p.Pos()

	imports := []*Name{}
	if p.Accept("import") {
		for {
			imports = append(imports, p.ParseName())
			if !p.Accept(",") {
				break
			}
		}
		p.Expect("in")
	}

	p.Expect("module")

	public := []*Binding{}
	for {
		public = append(public, p.ParseBinding())
		if !p.Accept(",") {
			break
		}
	}
	p.Expect("eof")

	return &Module{
		Imports:        imports,
		PublicBindings: public,
		Start:          start,
	}
}

func (p *Parser) ParseName() *Name {
	ident := p.Expect("identifier")
	return &Name{Value: ident.Value, Pos: ident.Pos}
}

func (p *Parser) ParseQualifiedName() Expression {
	ident := p.Expect("identifier")

	if p.Accept(".") {
		ident2 := p.Expect("identifier")
		return &QualifiedName{
			Module: ident.Value,
			Value:  ident2.Value,
			Start:  ident.Pos,
			End:    ident2.Pos,
		}
	}

	return &Name{Value: ident.Value, Pos: ident.Pos}
}

func (p *Parser) ParseExpression() Expression {

	if p.Is("let") {
		return p.ParseLet()
	}

	expr := p.ParseOperation()

	if p.Accept("->") {
		patt := ToPattern(expr)
		if patt == nil {
			source.Log("invalid pattern for lambda", expr.FirstPos().To(expr.LastPos()), source.SeverityError)
			panic("expect")
		}
		body := p.ParseExpression()
		expr = &Lambda{
			Cases: []*LambdaCase{{Pattern: patt, Expression: body}},
			Start: expr.FirstPos(),
			End:   p.PrevPos(),
		}

	}
	return expr
}

func (p *Parser) ParseLet() *Let {
	start := p.Pos()
	bindings := []*Binding{}

	p.Expect("let")
	for {
		bindings = append(bindings, p.ParseBinding())
		if !p.Accept(",") {
			break
		}
	}
	p.Expect("in")
	expr := p.ParseExpression()

	return &Let{Bindings: bindings, Expression: expr, Start: start}
}

func (p *Parser) ParseLambdaCase() *LambdaCase {

	expr := p.ParseOperation()
	patt := ToPattern(expr)

	if patt == nil {
		source.Log("invalid pattern for lambda", expr.FirstPos().To(p.Peek(-1).Pos), source.SeverityError)
		panic("expect")
	}

	p.Expect("->")

	body := p.ParseExpression()
	return &LambdaCase{Pattern: patt, Expression: body}
}

func (p *Parser) ParseBinding() *Binding {

	name := p.ParseName()
	p.Expect("=")
	expr := p.ParseExpression()

	return &Binding{Name: name, Expression: expr}
}

var operators = map[string]bool{
	">":  true,
	"<":  true,
	"*>": true,
	"<*": true,
}

func (p *Parser) ParseOperation() Expression {

	apps := []Expression{}

	apps = append(apps, p.ParseApplication())
	op := p.Peek(0).Value
	if operators[op] {
		for p.Accept(op) {
			apps = append(apps, p.ParseApplication())
		}
	}

	if len(apps) == 1 {
		return apps[0]
	}

	return &Operation{Operator: op, Operands: apps}
}

func (p *Parser) ParseApplication() Expression {

	atoms := []Expression{}
	atoms = append(atoms, p.ParseAtomic(true))

	for {
		atom := p.ParseAtomic(false)
		if atom == nil {
			break
		}
		atoms = append(atoms, atom)
	}

	if len(atoms) == 1 {
		return atoms[0]
	}

	return &Operation{Operator: "", Operands: atoms}
}

func (p *Parser) ParseAtomic(mandatory bool) Expression {
	start := p.Pos()

	if p.Is("identifier") {
		return p.ParseQualifiedName()
	}

	if p.Is("number") {
		tok := p.Consume()
		return &NumberLiteral{Value: tok.Number(), Pos: start}
	}

	if p.Is("string") {
		tok := p.Consume()
		return &StringLiteral{Value: tok.Value, Pos: start}
	}

	if p.Accept("(") {
		expr := p.ParseExpression()
		p.Expect(")")
		return expr
	}

	if p.Accept("[") {
		exprs := []Expression{}

		if p.Accept("]") {
			return &TupleExpr{SubExpressions: exprs, Start: start, End: p.PrevPos()}
		}

		exprs = append(exprs, p.ParseExpression())

		if p.Accept("]") {
			return &TupleExpr{SubExpressions: exprs, Start: start, End: p.PrevPos()}
		}

		if p.Accept(",") {
			for {
				exprs = append(exprs, p.ParseExpression())
				if !p.Accept(",") {
					break
				}
			}
			p.Expect("]")
			return &TupleExpr{SubExpressions: exprs, Start: start, End: p.PrevPos()}
		}

		if p.Accept(";") {
			if p.Accept("]") {
				return &List{SubExpressions: exprs, Start: start, End: p.PrevPos()}
			}

			for {
				exprs = append(exprs, p.ParseExpression())
				if !p.Accept(";") {
					break
				}
			}
			p.Expect("]")
			return &List{SubExpressions: exprs, Start: start, End: p.PrevPos()}
		}
	}

	if p.Accept("{") {
		cases := []*LambdaCase{}
		for {
			cases = append(cases, p.ParseLambdaCase())
			if !p.Accept(",") {
				break
			}
		}
		p.Expect("}")

		return &Lambda{Cases: cases, Start: start, End: p.PrevPos()}
	}

	if mandatory {
		source.Log("expected expression", p.Peek(0).Pos, source.SeverityError)
		panic("expect")
	}

	return nil
}

func ToPattern(expr Expression) Pattern {

	if name, ok := expr.(*Name); ok {
		return name
	}
	if number, ok := expr.(*NumberLiteral); ok {
		return number
	}
	if str, ok := expr.(*StringLiteral); ok {
		return str
	}

	if tup, ok := expr.(*TupleExpr); ok {
		subs := []Pattern{}
		for _, tsub := range tup.SubExpressions {
			tmp := ToPattern(tsub)
			if tmp == nil {
				return nil
			}
			subs = append(subs, tmp)
		}
		return &TuplePattern{SubPatterns: subs, Start: tup.Start, End: tup.End}
	}

	if list, ok := expr.(*List); ok {
		var subs []Pattern
		for _, sub := range list.SubExpressions {
			tmp := ToPattern(sub)
			if tmp == nil {
				return nil
			}
			subs = append(subs, tmp)
		}
		return &ListPattern{SubPatterns: subs, Start: list.Start, End: list.End}
	}

	return nil
}

// ----------- //

func Recover() {
	if r := recover(); r != nil {
		if r != "expect" {
			panic(r)
		}
	}
}

func ParseProgram(tokens []Token) *Program {
	defer Recover()

	parser := Parser{Tokens: tokens}
	return parser.ParseProgram()
}

func ParseModule(tokens []Token) *Module {
	defer Recover()

	parser := Parser{Tokens: tokens}
	return parser.ParseModule()
}

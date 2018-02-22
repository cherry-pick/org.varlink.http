package varlink

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

const (
	Bool = iota
	Int
	Float
	String
	Array
	Struct
	Alias
)

type TypeKind uint

type Type struct {
	Kind        TypeKind
	ElementType *Type
	Alias       string
	Fields      []TypeField
}

type TypeField struct {
	Name string
	Type *Type
}

type Interface struct {
	Name        string
	Description string

	Members []interface{}

	Aliases map[string]*TypeAlias
	Methods map[string]*Method
	Errors  map[string]*ErrorType
}

type TypeAlias struct {
	Name        string
	Description string
	Type        *Type
}

type Method struct {
	Name        string
	Description string
	In          *Type
	Out         *Type
}

type ErrorType struct {
	Name        string
	Description string
	Type        *Type // optional
}

type parser struct {
	input       string
	position    int
	lineStart   int
	lastComment bytes.Buffer
}

func (p *parser) next() int {
	r := -1

	if p.position < len(p.input) {
		r = int(p.input[p.position])
	}

	p.position += 1

	return r
}

func (p *parser) backup() {
	p.position -= 1
}

func (p *parser) advance() bool {
	for {
		char := p.next()

		if char == '\n' {
			p.lineStart = p.position
			p.lastComment.Reset()
		} else if char == ' ' {
			// ignore
		} else if char == '#' {
			p.next()
			start := p.position
			for {
				c := p.next()
				if c < 0 || c == '\n' {
					p.backup()
					break
				}
			}
			if p.lastComment.Len() > 0 {
				p.lastComment.WriteByte('\n')
			}
			p.lastComment.WriteString(p.input[start:p.position])
			p.next()
		} else {
			p.backup()
			break
		}
	}

	return p.position < len(p.input)
}

func (p *parser) advanceOnLine() {
	for {
		char := p.next()
		if char != ' ' {
			p.backup()
			return
		}
	}
}

func (p *parser) readKeyword() string {
	start := p.position

	for {
		char := p.next()
		if char < 'a' || char > 'z' {
			p.backup()
			break
		}
	}

	return p.input[start:p.position]
}

func (p *parser) readInterfaceName() string {
	start := p.position

	for {
		char := p.next()
		if (char < 'a' || char > 'z') && char != '-' && char != '.' {
			p.backup()
			break
		}
	}

	name := p.input[start:p.position]
	if len(name) < 3 || len(name) > 255 {
		return ""
	}

	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return ""
	}

	for _, part := range parts {
		if len(part) == 0 || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return ""
		}
	}

	return name
}

func (p *parser) readFieldName() string {
	start := p.position

	char := p.next()
	if (char < 'a' || char > 'z') && char != '_' {
		p.backup()
		return ""
	}

	for {
		char := p.next()
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			p.backup()
			break
		}
	}

	return p.input[start:p.position]
}

func (p *parser) readTypeName() string {
	start := p.position

	for {
		char := p.next()
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			p.backup()
			break
		}
	}

	return p.input[start:p.position]
}

func (p *parser) readStructType() *Type {
	if p.next() != '(' {
		p.backup()
		return nil
	}

	t := &Type{Kind: Struct}
	t.Fields = make([]TypeField, 0)

	char := p.next()
	if char != ')' {
		p.backup()

		for {
			field := TypeField{}

			p.advance()
			field.Name = p.readFieldName()
			if field.Name == "" {
				return nil
			}

			p.advance()
			if p.next() != ':' {
				return nil
			}

			p.advance()
			field.Type = p.readType()
			if field.Type == nil {
				return nil
			}

			t.Fields = append(t.Fields, field)

			p.advance()
			char = p.next()
			if char != ',' {
				break
			}
		}

		if char != ')' {
			return nil
		}
	}

	return t
}

func (p *parser) readType() *Type {
	var t *Type
	if keyword := p.readKeyword(); keyword != "" {
		switch keyword {
		case "bool":
			t = &Type{Kind: Bool}

		case "int":
			t = &Type{Kind: Int}

		case "float":
			t = &Type{Kind: Float}

		case "string":
			t = &Type{Kind: String}
		}
	} else if name := p.readTypeName(); name != "" {
		t = &Type{Kind: Alias, Alias: name}
	} else if t = p.readStructType(); t == nil {
		return nil
	}

	if p.next() == '[' {
		if p.next() != ']' {
			return nil
		}
		t = &Type{Kind: Array, ElementType: t}
	} else {
		p.backup()
	}

	return t
}

func (p *parser) readInterface() *Interface {
	if keyword := p.readKeyword(); keyword != "interface" {
		return nil
	}

	iface := &Interface{
		Members: make([]interface{}, 0),
		Aliases: make(map[string]*TypeAlias),
		Methods: make(map[string]*Method),
		Errors:  make(map[string]*ErrorType),
	}

	p.advance()
	iface.Description = p.lastComment.String()
	iface.Name = p.readInterfaceName()
	if iface.Name == "" {
		return nil
	}

	for {
		if !p.advance() {
			break
		}

		switch keyword := p.readKeyword(); keyword {
		case "type":
			alias := &TypeAlias{}

			p.advance()
			alias.Description = p.lastComment.String()
			alias.Name = p.readTypeName()
			if alias.Name == "" {
				return nil
			}

			p.advance()
			alias.Type = p.readType()
			if alias.Type == nil {
				return nil
			}

			iface.Members = append(iface.Members, alias)
			iface.Aliases[alias.Name] = alias

		case "method":
			method := &Method{}

			p.advance()
			method.Description = p.lastComment.String()
			method.Name = p.readTypeName()
			if method.Name == "" {
				return nil
			}

			p.advance()
			method.In = p.readType()
			if method.In == nil {
				return nil
			}

			p.advance()
			one := p.next()
			two := p.next()
			if (one != '-') || two != '>' {
				return nil
			}

			p.advance()
			method.Out = p.readType()
			if method.Out == nil {
				return nil
			}

			iface.Members = append(iface.Members, method)
			iface.Methods[method.Name] = method

		case "error":
			err := &ErrorType{}

			p.advance()
			err.Description = p.lastComment.String()
			err.Name = p.readTypeName()
			if err.Name == "" {
				return nil
			}

			p.advanceOnLine()
			err.Type = p.readType()

			iface.Members = append(iface.Members, err)
			iface.Errors[err.Name] = err

		default:
			return nil
		}
	}

	return iface
}

func (i *Interface) UnmarshalJSON(b []byte) error {
	var description string
	if err := json.Unmarshal(b, &description); err != nil {
		return err
	}

	iface := NewInterface(description)
	if iface == nil {
		return errors.New("invalid interface")
	}

	*i = *iface

	return nil
}

func writeComment(b *bytes.Buffer, comment string) {
	if len(comment) == 0 {
		return
	}

	for _, line := range strings.Split(comment, "\n") {
		b.WriteString("# ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func writeType(b *bytes.Buffer, t *Type, multiline bool) {
	switch t.Kind {
	case Bool:
		b.WriteString("bool")

	case Int:
		b.WriteString("int")

	case Float:
		b.WriteString("float")

	case String:
		b.WriteString("string")

	case Array:
		writeType(b, t.ElementType, multiline)
		b.WriteString("[]")

	case Struct:
		b.WriteString("(")
		for i, field := range t.Fields {
			if i > 0 {
				if multiline {
					b.WriteString(",")
				} else {
					b.WriteString(", ")
				}
			}
			if multiline {
				b.WriteString("\n  ")
			}
			b.WriteString(field.Name)
			b.WriteString(": ")
			writeType(b, field.Type, multiline)
		}
		if multiline {
			b.WriteString("\n")
		}
		b.WriteString(")")

	case Alias:
		b.WriteString(t.Alias)
	}
}

func (i *Interface) DefaultValue(t *Type) interface{} {
	switch t.Kind {
	case Bool:
		return false
	case Int:
		return 0
	case Float:
		return 0.0
	case String:
		return ""
	case Array:
		return make([]interface{}, 0)
	case Struct:
		v := make(map[string]interface{})
		for _, field := range t.Fields {
			v[field.Name] = i.DefaultValue(field.Type)
		}
		return v
	case Alias:
		alias := i.Aliases[t.Alias]
		if alias == nil {
			return nil
		}
		return i.DefaultValue(alias.Type)
	}
	return nil
}

func (i *Interface) String() string {
	var b bytes.Buffer

	writeComment(&b, i.Description)

	b.WriteString("interface ")
	b.WriteString(i.Name)

	for _, member := range i.Members {
		b.WriteString("\n\n")
		switch member.(type) {
		case *TypeAlias:
			alias := member.(*TypeAlias)
			writeComment(&b, alias.Description)
			b.WriteString("type ")
			b.WriteString(alias.Name)
			b.WriteString(" ")
			writeType(&b, alias.Type, true)

		case *Method:
			method := member.(*Method)
			writeComment(&b, method.Description)
			b.WriteString("method ")
			b.WriteString(method.Name)
			writeType(&b, method.In, false)
			b.WriteString(" -> ")
			writeType(&b, method.Out, false)

		case *ErrorType:
			err := member.(*ErrorType)
			writeComment(&b, err.Description)
			b.WriteString("error ")
			b.WriteString(err.Name)
			if err.Type != nil {
				b.WriteString(" ")
				writeType(&b, err.Type, true)
			}
		}
	}

	return b.String()
}

func NewInterface(description string) *Interface {
	p := &parser{input: description}

	p.advance()
	iface := p.readInterface()
	if iface == nil {
		return nil
	}

	if p.advance() {
		return nil
	}

	return iface
}

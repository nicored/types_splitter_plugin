package types_splitter_plugin

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// FileNamer is an interface for types that can return their name
type FileNamer interface {
	FileName() string
	Prefixed(prefix string) string
}

// Positioner is an interface for types that can return their position
type Positioner interface {
	Pos() *ast.Position
	ActualPos() *ast.Position
}

// Positioners is an interface for types that can return a list of Positioners
type Positioners interface {
	PosAfter(offset int) Positioners
}

// Shifter is an interface for types that can shift their position
type Shifter interface {
	Positioner
	ShiftOffset(offset, lines int)
}

// Shifters is an interface for types that can shift a list of Positioners
type Shifters interface {
	Positioners
	ShiftOffset(offset, lines int)
}

var (
	_ FileNamer = &Source{}

	_ Shifter  = &Definition{}
	_ Shifters = Definitions{}

	_ Shifter  = &FieldDefinition{}
	_ Shifters = FieldDefinitions{}
)

// Definition is a wrapper around ast.Definition that implements Positioner
// which is used for Query, Mutation, Subscription, and Type definitions
type Definition struct {
	*ast.Definition
	typ DefObjectType

	Content string
	Fields  FieldDefinitions

	// ActualPosition is the actual position of the definition in the source
	// including documentation and its entire scope
	ActualPosition *ast.Position
}

func WrapDefinition(def *ast.Definition, typ DefObjectType) *Definition {
	content, start, end, linesAbove := extractDefContent(typ, def)

	pos := &ast.Position{
		Src:   nil,
		Start: start,
		End:   end,
		Line:  def.Position.Line - linesAbove,
	}

	// check that we have the right position, we can't be off
	if def.Position.Src.Input[start:end+1] != content {
		panic(fmt.Sprintf("invalid definition content: %s != %s", def.Position.Src.Input[start:end+1], content))
	}

	return &Definition{
		Definition:     def,
		Content:        content,
		typ:            typ,
		ActualPosition: pos,
	}
}

func (d *Definition) AddFields(fields FieldDefinitions) {
	d.Fields = append(d.Fields, fields...)
}

// Pos returns the position of the definition including the source ref
func (d *Definition) Pos() *ast.Position {
	return d.Definition.Position
}

// ActualPos returns the actual position of the definition in the source
func (d *Definition) ActualPos() *ast.Position {
	return d.ActualPosition
}

// ShiftOffset shifts the position of the definition by the given offset and lines
func (d *Definition) ShiftOffset(offset, lines int) {
	shiftOffset(d, offset, lines)
}

// Definitions is a list of Definition
type Definitions []*Definition

// WrapDefinitions wraps a list of ast.Definition into a list of Definition
func WrapDefinitions(defList ast.DefinitionList, typ DefObjectType) Definitions {
	var defs = make(Definitions, 0, len(defList))

	for _, d := range defList {
		defs = append(defs, WrapDefinition(d, typ))
	}

	return defs
}

// PosAfter returns a list of Positioners that are defined after the given offset
func (ds Definitions) PosAfter(offset int) Positioners {
	var defs = make(Definitions, 0, len(ds))

	for _, d := range ds {
		if d.Pos().Start > offset {
			defs = append(defs, d)
		}
	}

	return defs
}

// ShiftOffset shifts the position of the definitions by the given offset and lines
func (ds Definitions) ShiftOffset(offset int, lines int) {
	for _, d := range ds {
		d.ShiftOffset(offset, lines)
	}
}

// FieldDefinition is a wrapper around ast.FieldDefinition that implements Positioner
// which is used for Field definitions in Query, Mutation, Subscription, and Type definitions
type FieldDefinition struct {
	*ast.FieldDefinition
	typ FieldDefType

	Content        string
	ActualPosition *ast.Position
}

func WrapFieldDefinition(field *ast.FieldDefinition, typ FieldDefType) *FieldDefinition {
	content, start, end, linesAbove := extractFieldDefContent(field)

	pos := &ast.Position{
		Src:   nil,
		Start: start,
		End:   end,
		Line:  field.Position.Line - linesAbove,
	}

	// check that we have the right position, we can't be off
	if field.Position.Src.Input[start:end+1] != content {
		panic(fmt.Sprintf("invalid field definition content: %s != %s", field.Position.Src.Input[start:end+1], content))
	}

	return &FieldDefinition{
		FieldDefinition: field,
		typ:             typ,
		Content:         content,
		ActualPosition:  pos,
	}
}

// Pos returns the position of the field definition including the source ref
func (fd *FieldDefinition) Pos() *ast.Position {
	return fd.FieldDefinition.Position
}

func (fd *FieldDefinition) ActualPos() *ast.Position {
	return fd.ActualPosition
}

// ShiftOffset shifts the position of the field definition by the given offset and lines
func (fd *FieldDefinition) ShiftOffset(offset, lines int) {
	shiftOffset(fd, offset, lines)
}

// FieldDefinitions is a list of FieldDefinition
type FieldDefinitions []*FieldDefinition

// PosAfter returns a list of Positioners that are defined after the given offset
func (ds FieldDefinitions) PosAfter(offset int) Positioners {
	var defs = make(FieldDefinitions, 0, len(ds))

	for _, d := range ds {
		if d.Pos().Start > offset {
			defs = append(defs, d)
		}
	}

	return defs
}

// ShiftOffset shifts the position of the FieldDefinitions by the given offset and lines
func (ds FieldDefinitions) ShiftOffset(offset int, lines int) {
	for _, d := range ds {
		d.ShiftOffset(offset, lines)
	}
}

// FindAstField returns the FieldDefinition that wraps the given ast.FieldDefinition
func (ds FieldDefinitions) FindAstField(field *ast.FieldDefinition) *FieldDefinition {
	for _, d := range ds {
		if d.FieldDefinition == field {
			return d
		}
	}

	return nil
}

// shiftOffset shifts the position of the given Positioner by the given offset and lines
func shiftOffset(pos Positioner, offset, lines int) {
	pos.Pos().Line -= lines
	pos.Pos().Start -= offset
	pos.Pos().End -= offset

	pos.ActualPos().Line -= lines
	pos.ActualPos().Start -= offset
	pos.ActualPos().End -= offset
}

func extractFieldDefContent(def *ast.FieldDefinition) (extracted string, actualStart, actualEnd int, abvLinesFound int) {
	input := def.Position.Src.Input
	start := def.Position.Start
	end := def.Position.End

	linesAbove, offsetBefore := findCommentStart(input, start)
	end = findCommentEnd(input, offsetBefore) + 1

	_, found := findNextOpenBracket('(', input, end, false)
	if found {
		if newEnd, ok := findClosingBracket('(', input, end, false); ok {
			end = newEnd
		}
	}

	end = findNextLineOffset(input, end)

	directives, newEnd := findLeftOverDirectives(input, end)
	if strings.TrimSpace(directives) != "" {
		end = newEnd
	}

	content, nBefore, _, spacesAfter := trimNewLines(input[offsetBefore : end+1])
	offsetBefore += nBefore
	end -= spacesAfter

	return content, offsetBefore, end, linesAbove - nBefore
}

func extractDefContent(typ DefObjectType, def *ast.Definition) (extracted string, actualStart int, actualEnd int, abvLinesFound int) {
	input := def.Position.Src.Input
	start := def.Position.Start
	end := def.Position.End

	linesAbove, offsetBefore := findCommentStart(input, start)

	if typ != DefScalar {
		if newEnd, found := findClosingBracket('{', input, start, false); found {
			end = newEnd
		}
	}

	end = findNextLineOffset(input, end)

	content, nBefore, _, spacesAfter := trimNewLines(input[offsetBefore : end+1])
	offsetBefore += nBefore
	end -= spacesAfter

	return content, offsetBefore, end, linesAbove
}

func findCommentStart(input string, startPos int) (linesFound, start int) {
	lines := 1

	startPos = findPrevLineOffset(input, startPos)

	// already starting with a comment
	if len(input) >= startPos+3 && input[startPos:startPos+3] == `"""` {
		return 0, start
	}

	enteredComment := false
	for i := startPos - 1; i >= 0; i-- {
		if input[i] == '\n' {
			lines++
			continue
		}

		if input[i] == '"' && i-2 >= 0 && input[i-1] == '"' && input[i-2] == '"' {
			if enteredComment {
				return lines, findPrevLineOffset(input, i) + 1
			}
			enteredComment = true
			i = i - 2
			continue
		}

		if input[i] != ' ' && input[i] != '\n' && input[i] != '\t' {
			if !enteredComment {
				return lines, startPos + 1
			}
		}
	}

	return 0, findPrevLineOffset(input, startPos) + 1
}

func findCommentEnd(input string, startPos int) (end int) {
	originalStartPos := startPos

	enteredComment := false
	for i := startPos; i < len(input); i++ {
		if input[i] == '"' && i+2 < len(input) && input[i+1] == '"' && input[i+2] == '"' {
			if enteredComment {
				return i + 3
			}
			enteredComment = true
			i = i + 2
			continue
		}

		if input[i] != ' ' && input[i] != '\n' && input[i] != '\t' {
			if !enteredComment {
				return originalStartPos
			}
		}
	}

	return originalStartPos
}

func findLeftOverDirectives(input string, start int) (string, int) {
	spaces := 0
	for i := start; i < len(input); i++ {
		if input[i] == ' ' || input[i] == '\n' || input[i] == '\t' {
			spaces++
			continue
		}

		if input[i] != '@' {
			return input[start : i-spaces], i - spaces - 1
		}

		if input[i] == '@' {
			spaces = 0
			i = findNextLineOffset(input, i)
			continue
		}
	}
	return "", start
}

func findPrevLineOffset(input string, start int) int {
	for i := start - 1; i >= 0; i-- {
		if input[i] == '\n' {
			return i
		}
	}
	return start
}

func findNextLineOffset(input string, start int) int {
	for i := start; i < len(input); i++ {
		if input[i] == '\n' {
			return i
		}
	}
	return start
}

const (
	OpenParen  = '('
	CloseParen = ')'

	OpenBrace  = '{'
	CloseBrace = '}'

	OpenSqBracket  = '['
	CloseSqBracket = ']'
)

func closingBracket(bracket rune) rune {
	switch bracket {
	case OpenParen:
		return CloseParen
	case OpenBrace:
		return CloseBrace
	case OpenSqBracket:
		return CloseSqBracket
	}
	return 0
}

func findNextOpenBracket(bracket rune, input string, start int, multiLine bool) (offset int, found bool) {
	for i := start; i < len(input); i++ {
		if input[i] == '\n' && !multiLine {
			return start, false
		}

		if rune(input[i]) == bracket {
			return i, true
		}
	}
	return start, false
}

func findClosingBracket(openBracket rune, input string, start int, startInsideScope bool) (offset int, found bool) {
	closeBracket := closingBracket(openBracket)

	brackets := -1
	if startInsideScope {
		brackets = 0
	}
	inQuote := false
	quote := '"'
	inComment := false

	for i := start; i < len(input); i++ {
		if inComment && len(input) >= i+3 && input[i:i+3] != `"""` || inQuote && rune(input[i]) != quote && rune(input[i-1]) != '\\' {
			continue
		}

		if len(input) >= i+3 && input[i:i+3] == `"""` {
			inComment = !inComment
			i = i + 2
			continue
		}
		if rune(input[i]) == closeBracket && !inQuote && !inComment {
			if brackets == 0 {
				return i, true
			}
			brackets--
			continue
		}

		if rune(input[i]) == openBracket && !inQuote && !inComment {
			brackets++
			continue
		}

		if rune(input[i]) == '\'' || rune(input[i]) == '"' {
			if !inQuote {
				inQuote = true
				quote = rune(input[i])
				continue
			}
			if quote == rune(input[i]) && rune(input[i-1]) != '\\' {
				inQuote = false
			}
			continue
		}

		if rune(input[i]) == '"' && !inQuote {
			inQuote = !inQuote
		}
	}

	return len(input), false
}

func trimNewLines(input string) (str string, linesBefore, linesAfter, spacesAfter int) {
	for i := 0; i < len(input); i++ {
		if input[i] == '\n' {
			continue
		}

		linesBefore = i
		break
	}

	for i := len(input) - 1; i >= 0; i-- {
		if input[i] == '\n' {
			linesAfter++
			spacesAfter++
			continue
		}

		if input[i] == ' ' || input[i] == '\t' {
			spacesAfter++
		}

		break
	}

	return input[linesBefore : len(input)-spacesAfter], linesBefore, linesAfter, spacesAfter
}

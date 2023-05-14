package types_splitter_plugin

import (
	"bytes"
	_ "embed"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/vektah/gqlparser/v2/ast"
)

var (
	//go:embed tpl/extended_query_type.go.tmpl
	tmplQueryExtended string
	//go:embed tpl/query_type.go.tmpl
	tmplQuery string
	//go:embed tpl/type_definitions.go.tmpl
	tmplObject string
)

// Source is a wrapper around ast.Source that implements Positioner
type Source struct {
	*ast.Source
	typ SourceType

	Fields FieldDefinitions
	Types  Definitions

	isMainQuery bool
}

// Sources is a list of Source
type Sources []*Source

// SourcesDefs is a map of source names to Definitions
type SourcesDefs map[string]Definitions

// SourcesFields is a map of source names to FieldDefinitions
type SourcesFields map[string]FieldDefinitions

// SourcesMap is a map of source names to Sources
type SourcesMap map[string]*Source

// WrapSource wraps an ast.Source in a Source
func WrapSource(s *ast.Source) *Source {
	return &Source{
		Source: s,
		typ:    OriginalSource,
	}
}

type QueryViewData struct {
	Type   string
	Fields FieldDefinitions
}

type TypeViewData struct {
	Type  string
	Types Definitions
}

// NewSource creates a new Source
func NewSource(name string, sourceType SourceType) (*Source, error) {
	src := &Source{
		Source: &ast.Source{
			Name:    name,
			Input:   "",
			BuiltIn: false,
		},
		typ: sourceType,
	}

	input, err := src.GenerateInput()
	if err != nil {
		return nil, err
	}

	src.Input = input

	return src, nil
}

func (s *Source) GenerateInput() (string, error) {
	isQueryType := true
	typeName := ""
	switch s.typ {
	case SourceQueryExtended:
		typeName = "Query"
	case SourceMutationExtended:
		typeName = "Mutation"
	case SourceSubscriptionExtended:
		typeName = "Subscription"
	case OriginalSource:
		return "", fmt.Errorf("cannot generate input for original source")
	default:
		isQueryType = false
	}

	writer := bytes.Buffer{}
	if isQueryType {
		tmpl := tmplQueryExtended
		if s.isMainQuery {
			tmpl = tmplQuery
		}

		tpl, err := template.New("query_extended").Parse(tmpl)
		if err != nil {
			return "", err
		}

		if err = tpl.Execute(&writer, QueryViewData{
			Type:   typeName,
			Fields: s.Fields,
		}); err != nil {
			return "", err
		}
	} else {
		tpl, err := template.New("object").Parse(tmplObject)
		if err != nil {
			return "", err
		}

		if err = tpl.Execute(&writer, TypeViewData{
			Types: s.Types,
		}); err != nil {
			return "", err
		}
	}

	s.Input = writer.String()

	return writer.String(), nil
}

// FileName returns the name of the source
func (s *Source) FileName() string {
	return s.Name
}

// Prefixed adds a prefix to the name of the source and returns it
func (s *Source) Prefixed(prefix string) string {
	return filepath.Join(filepath.Dir(s.Name), prefix+"."+filepath.Base(s.Name))
}

// WrapSources wraps a list of ast.Source into a list of Source
func WrapSources(sources []*ast.Source) Sources {
	var wrapped = make(Sources, 0, len(sources))

	for _, s := range sources {
		wrapped = append(wrapped, WrapSource(s))
	}

	return wrapped
}

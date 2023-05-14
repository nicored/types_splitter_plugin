package types_splitter_plugin

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/vektah/gqlparser/v2/ast"
)

const (
	PluginName      = "types_splitter"
	ResolversSuffix = ".resolvers"
)

type DefObjectType uint32

const (
	DefQueryObject DefObjectType = iota
	DefMutationObject
	DefSubscriptionObject
	DefTypeObject
	DefInputObject
	DefScalar
)

type FieldDefType uint32

const (
	DefQueryField FieldDefType = iota
	DefMutationField
	DefSubscriptionField
	DefObjectField
)

type SourceType uint32

const (
	OriginalSource SourceType = iota
	SourceQueryExtended
	SourceMutationExtended
	SourceSubscriptionExtended
	SourceObject
	SourceInput
)

// TypesSplitterPlugin is a plugin that splits the schema into multiple files.
type TypesSplitterPlugin struct {
	genCfg *config.Config
	cfg    *SplitterConfig

	// sources is a map of existing source name to source
	sources SourcesMap
	// sourcesDefs is a map of existing source name to a list of Definition that are defined in that source
	sourcesDefs SourcesDefs
	// sourcesFields is a map of existing source name to a list of FieldDefinition that are fields in that source
	sourcesFields SourcesFields
	// sourcesFieldsIndex is a map of existing source name to a map of FieldDefinition to index in the list of FieldDefinition
	sourcesFieldsIndex map[string]map[*ast.FieldDefinition]int

	// sources is a map of new source name to source
	newSources SourcesMap
	// newSourcesDef is a map of new source name to a list of Definition that are defined in that source
	newSourcesDef SourcesDefs
}

// New creates a new TypesSplitterPlugin.
func New(cfgFilePath string) (*TypesSplitterPlugin, error) {
	cfg, err := loadConfig(cfgFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return &TypesSplitterPlugin{
		cfg: cfg,
	}, nil
}

func (s *TypesSplitterPlugin) init(genCfg *config.Config) {
	s.genCfg = genCfg

	// override the default resolver template config
	s.genCfg.Resolver.FilenameTemplate = "{name}" + ResolversSuffix + ".go"
	s.genCfg.Resolver.Layout = "follow-schema"

	s.newSources = make(SourcesMap)
	s.newSourcesDef = make(SourcesDefs)
	s.sourcesFieldsIndex = make(map[string]map[*ast.FieldDefinition]int)

	s.initSources(genCfg)
}

func (s *TypesSplitterPlugin) initSources(genCfg *config.Config) {
	s.sources = make(SourcesMap)
	s.sourcesDefs = make(SourcesDefs, len(genCfg.Sources))
	s.sourcesFields = make(SourcesFields)

	for _, cfgSource := range genCfg.Sources {
		source := WrapSource(cfgSource)
		s.sources[source.Source.Name] = source

		// types
		srcTypes, srcTypesFields := s.getSourceDefs(source, mapToList(genCfg.Schema.Types), DefTypeObject)
		s.sourcesFields[source.Source.Name] = append(s.sourcesFields[source.Source.Name], srcTypesFields...)
		s.sourcesDefs[source.Source.Name] = append(s.sourcesDefs[source.Source.Name], srcTypes...)
		source.Types = append(source.Types, srcTypes...)

		// queries
		srcQueries, srcQueriesFields := s.getSourceDefs(source, []*ast.Definition{genCfg.Schema.Query}, DefQueryObject)
		s.sourcesFields[source.Source.Name] = append(s.sourcesFields[source.Source.Name], srcQueriesFields...)
		s.sourcesDefs[source.Source.Name] = append(s.sourcesDefs[source.Source.Name], srcQueries...)

		// mutations
		srcMutations, srcMutationsFields := s.getSourceDefs(source, []*ast.Definition{genCfg.Schema.Mutation}, DefMutationObject)
		s.sourcesFields[source.Source.Name] = append(s.sourcesFields[source.Source.Name], srcMutationsFields...)
		s.sourcesDefs[source.Source.Name] = append(s.sourcesDefs[source.Source.Name], srcMutations...)

		// subscriptions
		srcSubscriptions, srcSubscriptionsFields := s.getSourceDefs(source, []*ast.Definition{genCfg.Schema.Subscription}, DefSubscriptionObject)
		s.sourcesFields[source.Source.Name] = append(s.sourcesFields[source.Source.Name], srcSubscriptionsFields...)
		s.sourcesDefs[source.Source.Name] = append(s.sourcesDefs[source.Source.Name], srcSubscriptions...)
	}
}

// Name implements plugin.Plugin
func (s *TypesSplitterPlugin) Name() string {
	return PluginName
}

// MutateConfig implements plugin.ConfigMutator
func (s *TypesSplitterPlugin) MutateConfig(genCfg *config.Config) error {
	s.init(genCfg)

	// mutate and extend queries, mutations and subscriptions based on QueryConfig
	if len(s.cfg.QueryConfig) > 0 {
		if err := s.mutateQueryTypes(); err != nil {
			return fmt.Errorf("failed to mutate query: %w", err)
		}
	}

	// mutate object types based on TypeConfig
	if len(s.cfg.TypeConfig) > 0 {
		if err := s.mutateObjectTypes(); err != nil {
			return fmt.Errorf("failed to mutate object types: %w", err)
		}
	}

	for _, newSrc := range s.newSources {
		_, err := newSrc.GenerateInput()
		if err != nil {
			return err
		}

		// remove extra newlines
		newSrc.Input = removeExtraLines(newSrc.Input)

		// add the new sources to the config
		genCfg.Sources = append(genCfg.Sources, newSrc.Source)
	}

	// reorder sources by name
	sort.Slice(genCfg.Sources, func(i, j int) bool {
		return genCfg.Sources[i].Name < genCfg.Sources[j].Name
	})

	return nil
}

// mutateObjectTypes mutates the object types based on the TypeConfig
func (s *TypesSplitterPlugin) mutateObjectTypes() error {
	var err error

	for sourceName, definitions := range s.sourcesDefs {
		for _, def := range definitions {
			// we're not handing Input types...
			// we possibly could pretty easily from here. It might be as easy as adding
			// && def.typ != DefTypeInput to the if statement below. The content added to the source
			// should already be generated at this point in the process, and the template uses .Content from
			// the definition. So, easy as that? Maybe?
			if def.typ != DefTypeObject {
				continue
			}

			prefix, ok := s.cfg.TypeConfig.FindResolverPrefix(def.Name)
			if !ok {
				continue
			}

			// the new source name is the prefix from config + the original source name
			newSrcName := filepath.Join(filepath.Dir(sourceName), prefix+filepath.Ext(sourceName))

			// check if the new source conflicts with an existing source
			if sourceName == newSrcName {
				continue
			}

			// check if that source exists
			newExistingSrc, ok := s.newSources[newSrcName]
			if !ok {
				if newExistingSrc, err = NewSource(newSrcName, SourceObject); err != nil {
					return err
				}
				s.newSources[newSrcName] = newExistingSrc
			}

			// add the type to the new source
			newExistingSrc.Types = append(newExistingSrc.Types, def)
			def.ActualPosition.Src = newExistingSrc.Source

			// remove the field from the original query and source file (from original config)
			if err = s.moveType(s.sources[sourceName], def); err != nil {
				return err
			}
		}
	}

	return nil
}

// moveType moves a type from one source to another by removing it from the source it's in and adding it to the new source.
// More info in the comments below and in moveQueryType.
func (s *TypesSplitterPlugin) moveType(fromSource *Source, typeToMove *Definition) error {
	for _, cfgType := range fromSource.Types {
		if cfgType.Name == typeToMove.Name {
			cfgSrc := typeToMove.Pos().Src

			// get the next types in the list and sort them by position (should already be sorted, but...)
			types := s.sourcesDefs[cfgSrc.Name].PosAfter(typeToMove.ActualPosition.End)
			sortedTypes := types.(Definitions)
			sort.Slice(sortedTypes, func(i, j int) bool {
				return sortedTypes[i].Position.Start < sortedTypes[j].Position.Start
			})

			offset := 0
			offsetLine := 0

			// we calculate the offset between the type to remove and the next field so that
			// we can shift all next positions accordingly
			if len(sortedTypes) > 0 {
				offset = sortedTypes[0].ActualPosition.Start - typeToMove.ActualPosition.Start
				offsetLine = countLines(cfgSrc.Input[typeToMove.ActualPosition.Start:sortedTypes[0].ActualPosition.Start])
			}

			// debug
			before := cfgSrc.Input

			removed := ""
			if len(sortedTypes) > 0 {
				// debug
				removed = cfgSrc.Input[typeToMove.ActualPosition.Start:sortedTypes[0].ActualPosition.Start]

				// remove field from source input
				cfgSrc.Input = cfgSrc.Input[:typeToMove.ActualPosition.Start] + cfgSrc.Input[sortedTypes[0].ActualPosition.Start:]
			} else {
				// debug
				removed = cfgSrc.Input[typeToMove.ActualPosition.Start:]

				// remove type from source input
				cfgSrc.Input = cfgSrc.Input[:typeToMove.ActualPosition.Start] + cfgSrc.Input[typeToMove.ActualPosition.End+1:]

				// remove extra newlines
				cfgSrc.Input = removeExtraLines(cfgSrc.Input)
			}

			// debug
			after := cfgSrc.Input

			// debug
			newDebSrcCh(
				cfgSrc.Name,
				typeToMove.Name,
				before,
				removed,
				after,
				typeToMove.ActualPosition.Start,
				typeToMove.ActualPosition.End,
			)

			// shift offset for all next fields
			for _, typedef := range sortedTypes {
				typedef.ShiftOffset(offset, offsetLine)

				for _, field := range typedef.Fields {
					field.ShiftOffset(offset, offsetLine)
				}
			}

			// we can now update the source of the moved field
			typeToMove.Position.Src = typeToMove.ActualPosition.Src
			for _, field := range typeToMove.Fields {
				field.Position.Src = typeToMove.Position.Src
			}

			return nil
		}
	}
	return nil
}

// mutateQueryTypes mutates the query, mutation and subscription types
func (s *TypesSplitterPlugin) mutateQueryTypes() error {
	var err error

	for sourceName, fields := range s.sourcesFields {
		for _, field := range fields {
			if !isQueryTypeField(field) {
				continue
			}

			// any field not found in the query config is ignored
			// and will kept in the root query source
			prefix, ok := s.cfg.QueryConfig.FindResolverPrefix(field.Name)
			if !ok {
				continue
			}

			// origQuery is the original query, mutation or subscription that contains the field
			var origQuery *ast.Definition
			var sourceType SourceType

			switch field.typ {
			case DefQueryField:
				origQuery = s.genCfg.Schema.Query
				sourceType = SourceQueryExtended
			case DefMutationField:
				origQuery = s.genCfg.Schema.Mutation
				sourceType = SourceMutationExtended
			case DefSubscriptionField:
				origQuery = s.genCfg.Schema.Subscription
				sourceType = SourceSubscriptionExtended
			}

			// the new source name is the prefix from config + the original source name
			newSrcName := filepath.Join(filepath.Dir(sourceName), prefix+"."+filepath.Base(sourceName))

			// check if the new source conflicts with an existing source
			if sourceName == newSrcName {
				continue
			}

			// check if that source exists
			newExistingSrc, ok := s.newSources[newSrcName]
			if !ok {
				if newExistingSrc, err = NewSource(newSrcName, sourceType); err != nil {
					return err
				}
				s.newSources[newSrcName] = newExistingSrc
			}

			// add the field to the new source so we can generate the content later
			newExistingSrc.Fields = append(newExistingSrc.Fields, field)
			field.ActualPosition.Src = newExistingSrc.Source

			// remove the field from the original query and source file (from original config)
			_, err := s.moveQueryField(origQuery, field)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// moveQueryField updates the Definition fields list, and removes the field definition from the
// graphql source file (in memory, from the original gqlgen config).
//
// This function is intended to be used for Query, Mutation and Subscription fields, as we need to move them to
// extensions of the definition type.
//
// Note that updating the source file is because during compile time, the server will try to load graphql schema
// (from generated go code) and will complain that duplicate types exist in the schema.
//
// Also note that this function could be written more efficiently, but given that its only purpose is to be used
// during code generation, it's not worth the effort.
func (s *TypesSplitterPlugin) moveQueryField(fromOriginalDef *ast.Definition, fieldToMove *FieldDefinition) (bool, error) {
	for _, cfgQField := range fromOriginalDef.Fields {
		if strings.HasPrefix(cfgQField.Name, "__") || cfgQField.Position == nil {
			continue
		}

		cfgSrc := fieldToMove.Pos().Src

		if cfgQField == fieldToMove.FieldDefinition {
			// get the next fields in the list and sort them by position (should already be sorted but
			// it's 5 more minutes I don't want to spend on this.
			fields := s.sourcesFields[cfgSrc.Name].PosAfter(fieldToMove.ActualPosition.End)
			sortedFields := fields.(FieldDefinitions)
			sort.Slice(sortedFields, func(i, j int) bool {
				return sortedFields[i].Position.Start < sortedFields[j].Position.Start
			})

			offset := 0
			offsetLine := 0

			// we calculate the offset between the field to remove and the next field so that
			// we can shift all next positions accordingly
			if len(sortedFields) > 0 {
				offset = sortedFields[0].ActualPosition.Start - fieldToMove.ActualPosition.Start
				offsetLine = countLines(cfgSrc.Input[fieldToMove.ActualPosition.Start:sortedFields[0].ActualPosition.Start])
			}

			// debug
			before := cfgSrc.Input

			removed := ""
			if len(sortedFields) > 0 {
				// debug
				removed = cfgSrc.Input[fieldToMove.ActualPosition.Start:sortedFields[0].ActualPosition.Start]

				// remove field from source input
				cfgSrc.Input = cfgSrc.Input[:fieldToMove.ActualPosition.Start] + cfgSrc.Input[sortedFields[0].ActualPosition.Start:]
			} else {
				// debug
				removed = cfgSrc.Input[fieldToMove.ActualPosition.Start : fieldToMove.ActualPosition.End+1]

				// remove field from source input
				cfgSrc.Input = cfgSrc.Input[:fieldToMove.ActualPosition.Start] + cfgSrc.Input[fieldToMove.ActualPosition.End+1:]

				// remove extra newlines
				cfgSrc.Input = removeExtraLines(cfgSrc.Input)
			}

			// debug
			after := cfgSrc.Input

			// debug
			newDebSrcCh(
				cfgSrc.Name,
				fieldToMove.Name,
				before,
				removed,
				after,
				fieldToMove.ActualPosition.Start,
				fieldToMove.ActualPosition.End,
			)

			// shift offset for all next fields
			sortedFields.ShiftOffset(offset, offsetLine)

			// we can now update the source of the moved field
			fieldToMove.Position.Src = fieldToMove.ActualPosition.Src

			// check whether the source is empty, and if so, remove it from the list.
			// Note: an empty source is a source that has no fields referencing it.
			// For Query/Mutation/Subscription, we don't remove the fields from the object, we only
			// point fields to a different source.
			for _, field := range s.sourcesFields[cfgSrc.Name] {
				// if the source of a field is the same as the source, then
				// the query is not empty
				if field.Position.Src.Name == cfgSrc.Name {
					return false, nil
				}
			}

			// remove the source from the config sources
			for i, src := range s.genCfg.Sources {
				if src.Name == cfgSrc.Name {
					s.genCfg.Sources = append(s.genCfg.Sources[:i], s.genCfg.Sources[i+1:]...)
					break
				}
			}

			// update the main query source to be the source of the first field of the same source type.
			// This will be used to generate the schema so that the main query source is not an extended type.
			firstField := s.sourcesFields[cfgSrc.Name][0]
			s.newSources[firstField.Position.Src.Name].isMainQuery = true

			switch firstField.typ {
			case DefQueryField:
				s.genCfg.Schema.Query.Position.Src = firstField.Position.Src
			case DefMutationField:
				s.genCfg.Schema.Mutation.Position.Src = firstField.Position.Src
			case DefSubscriptionField:
				s.genCfg.Schema.Subscription.Position.Src = firstField.Position.Src
			default:
				return false, fmt.Errorf("unknown field type %d", firstField.typ)
			}

			// remove the source from the list
			delete(s.sourcesFields, cfgSrc.Name)

			return true, nil
		}
	}

	return false, nil
}

func (s *TypesSplitterPlugin) getSourceDefs(src *Source, srcDefs ast.DefinitionList, typ DefObjectType) (Definitions, FieldDefinitions) {
	defs := Definitions{}
	fields := FieldDefinitions{}

	var fieldTyp FieldDefType
	switch typ {
	case DefTypeObject:
		fieldTyp = DefObjectField
	case DefQueryObject:
		fieldTyp = DefQueryField
	case DefMutationObject:
		fieldTyp = DefMutationField
	case DefSubscriptionObject:
		fieldTyp = DefSubscriptionField
	}

	for _, srcDef := range srcDefs {
		defFields := FieldDefinitions{}

		if srcDef == nil || srcDef.Position == nil {
			continue
		}
		if srcDef.Position.Src != src.Source {
			continue
		}

		if typ == DefTypeObject && isQueryDef(srcDef) {
			continue
		}

		if typ == DefTypeObject && srcDef.Kind == "INPUT_OBJECT" {
			if srcDef.Kind == "INPUT_OBJECT" {
				typ = DefInputObject
			} else if srcDef.Kind == "SCALAR" {
				typ = DefScalar
			}
		}

		def := WrapDefinition(srcDef, typ)
		defs = append(defs, def)

		for _, field := range srcDef.Fields {
			if field == nil || field.Position == nil || typ == DefScalar {
				continue
			}
			defFields = append(defFields, WrapFieldDefinition(field, fieldTyp))
		}

		def.AddFields(defFields)
		fields = append(fields, defFields...)
	}

	return defs, fields
}

func isQueryDef(def *ast.Definition) bool {
	return def.Name == "Query" || def.Name == "Mutation" || def.Name == "Subscription"
}

func isQueryTypeField(field *FieldDefinition) bool {
	return field.typ == DefQueryField || field.typ == DefMutationField || field.typ == DefSubscriptionField
}

var (
	manyLinesRegex      = regexp.MustCompile("\n{2,}")
	startLineRegex      = regexp.MustCompile("^\n+")
	linesToClosureRegex = regexp.MustCompile("\n{2,}(\\s*})")
	endLineRegex        = regexp.MustCompile("\n+$")
)

func removeExtraLines(str string) string {
	str = manyLinesRegex.ReplaceAllString(str, "\n\n")
	str = startLineRegex.ReplaceAllString(str, "")
	str = linesToClosureRegex.ReplaceAllString(str, "\n$1")
	str = endLineRegex.ReplaceAllString(str, "\n")
	return str
}

// shows space characters in output for debugging
func debf(str string) string {
	str = strings.Replace(str, "\n", "\n\\n", -1)
	str = strings.Replace(str, "\t", "\\t\t", -1)
	str = strings.Replace(str, " ", "[ ]", -1)
	return str
}

// debug
type debSrcChange struct {
	n       int
	source  string
	def     string
	before  string
	removed string
	after   string
	start   int
	end     int
}

func countLines(s string) int {
	return strings.Count(s, "\n")
}

func mapToList[T any](m map[string]T) []T {
	l := make([]T, len(m))
	i := 0
	for _, v := range m {
		l[i] = v
		i++
	}

	return l
}

// debug
var debSources []debSrcChange
var debSourcesDetailed []debSrcChange

// debug
func newDebSrcCh(source, def, before, removed, after string, start, end int) {
	debSources = append(debSources, debSrcChange{
		n:       len(debSources),
		source:  source,
		def:     def,
		before:  before,
		removed: removed,
		after:   after,
		start:   start,
		end:     end,
	})

	debSourcesDetailed = append(debSourcesDetailed, debSrcChange{
		n:       len(debSourcesDetailed),
		source:  source,
		def:     def,
		before:  debf(before),
		removed: debf(removed),
		after:   debf(after),
		start:   start,
		end:     end,
	})
}

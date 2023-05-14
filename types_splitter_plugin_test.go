package types_splitter_plugin

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func Test_MutateConfig(t *testing.T) {
	sources := getTestSources(t, false)
	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sources: sources,
		Schema:  schema,
	}

	splitter, err := New("./test_data/gqlgen_plugins.yml")
	if err != nil {
		t.Fatal(err)
	}

	if err = splitter.MutateConfig(cfg); err != nil {
		t.Fatal(err)
	}

	expected := getTestSources(t, true)
	if len(expected) != len(cfg.Sources) {
		t.Fatalf("expected %d sources, got %d", len(expected), len(cfg.Sources))
	}

	for i, src := range cfg.Sources {
		if src.Name != expected[i].Name {
			t.Errorf("expected source name %s, got %s", expected[i].Name, src.Name)
		}
		if src.Input != expected[i].Input {
			t.Errorf("expected source input %s, got %s", expected[i].Input, src.Input)
		}
	}
}

func getTestSources(t *testing.T, isExpected bool) []*ast.Source {
	t.Helper()

	var srcList []*ast.Source

	var dir = "test_data/gql_input"
	if isExpected {
		dir = "test_data/gql_expected"
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".graphql") {
			continue
		}

		src, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}

		srcList = append(srcList, &ast.Source{
			Name:  file.Name(),
			Input: string(src),
		})
	}

	return srcList
}

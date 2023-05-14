# TypesSplitterPlugin [![Integration](https://github.com/nicored/types_splitter_plugin/actions/workflows/integration.yml/badge.svg)](https://github.com/nicored/types_splitter_plugin/actions) [![Coverage Status](https://coveralls.io/repos/github/nicored/types_splitter_plugin/badge.svg?branch=main)](https://coveralls.io/github/nicored/types_splitter_plugin?branch=main) [![Go Report Card](https://goreportcard.com/badge/github.com/nicored/types_splitter_plugin)](https://goreportcard.com/report/github.com/nicored/types_splitter_plugin) [![Go Reference](https://pkg.go.dev/badge/github.com/nicored/types_splitter_plugin.svg)](https://pkg.go.dev/github.com/nicored/types_splitter_plugin)

This plugin is used to split the generated types into multiple files.

## Usage

### Configuration

You need a Yaml configuration file in your project, in this example we will call it `gqlgen_plugins.yml`.

```yaml
types_splitter:
  types:
    -
      name: Posts
      prefix: posts
    -
      name: Manager
      prefix: managers.users
    -
      name: User
      prefix: users
  queries:
    -
      prefix: posts
      matches:
        - post
    -
      prefix: users
      matches:
        - user|manager
    -
      prefix: editors
      matches:
        - editor
```

- `types_splitter` is the name of the plugin


- `types` is the list of types to split
  - `name` is the name of the type
  - `prefix` is the prefix we want to apply to the generated file. eg. `managers.users` will generate `managers.users.resolvers.go`


- `queries` is the list of queries to split (queries being Query, Mutation, Subscription types)
  - `prefix` is the prefix we want to apply to the generated file. eg. `users` will generate `users.queries.resolvers.go` or `users.mutations.resolvers.go` should their be any matches.
  - `matches` is the list of queries to match. eg. `user|manager` will match `user` and `manager` queries (ie. `getUser`).

Note that the order of the `types` and `queries` is important as the first match will be used.

### Custom plugin

One way to use the plugin is to create a custom plugin that will load the configuration file and pass it to the plugin.

Create a new file called `generate.go` where your `resolvers.go` live, and add the following code:

```go
//go:build ignore
package main

import (
    "github.com/99designs/gqlgen/api"
    "github.com/99designs/gqlgen/codegen/config"
    "github.com/99designs/gqlgen/plugin"
    splitter "github.com/nicored/types_splitter_plugin"
)

func main() {
    cfg, err := config.LoadConfigFromDefaultLocations()
    if err != nil {
        panic(err)
    }

    tsPluging, err := splitter.New("gqlgen_plugins.yml")
	if err != nil {
		panic(err)
    }

    err = api.Generate(cfg,
        api.AddPlugin(tsPluging),
    )
    if err != nil {
        panic(err)
    }
}
```

Then in your `resolvers.go` file, add the following comment:

```go
//go:generate go run generate.go
```

And run it with `go generate ./...`

## How it works

The plugin will go through all the types and queries and will generate a map of types and fields to files.

Then based on your configuration, it will determine whether to split the type or query into a new file, or keep it in its original file.

If an original source file is emptied after the split, it will be deleted.

If a query type (Query, Mutation, Subscription) is emptied after the split, it will be deleted and the source of the first definition of the query will become the main source file (eg. `type Query` instead of an extended type `extend type Query`)

## Limitations

I made this plugin for my own use, so you may experience issues with it depending on your use case:

**OS**

It doesn't currently work on Windows:

- [ ] Fix for Windows

**Types**

The following splits aren't supported, but could easily be added if needed:

- [ ] Interfaces are left out
- [ ] Enums are left out
- [ ] Input types are left out
- [ ] Scalars are left out
- [ ] Unions are left out

**Parsing:**

I used the data from Config.Schema, and parsed the content in a rather naive way, so if you have a complex schema, it may not work as expected.

The found that the parser doesn't output consistent results for Position. Comments on fields are not included in the Position data, but comments on types are. So I had to use a workaround to get the correct position starting from the comment to the end of the field/type definition.

- [ ] Use a lexer/parser to parse source input
- [ ] The approach to move type definitions and fields to their new source is rather naive, and may lead to issues with complex schema. 
- [ ] Comments with "#" may not be parsed and moved correctly.
- [ ] Removal of extra lines could lead to issues as the approach is also rather naive.

## Contributing

Contributions are welcomed, send an MR or open an issue if you find a bug or want to add a feature.

For instance, the ability to set what the main source file for query types should be would be a nice addition ;)

- [ ] Ability to set the main source file for query types
- [ ] More unit-tests

## License
MIT

# chaingen

Chaingen is a golang code generation tool which lets you to create composite builders with chained calls. It's easier to understand it's purpose through example.

## Problem

Let's create very simple builder of imaginary SQL language:

```go
package main

import (
	"fmt"
	"strings"
)

type WhereBuilder struct {
	conditions []string
}

func (w WhereBuilder) Where(condition string) WhereBuilder {
	w.conditions = append(w.conditions, condition)
	return w
}

func (w WhereBuilder) Build() string {
	return fmt.Sprintf("WHERE %s", strings.Join(w.conditions, " AND "))
}

type OffsetBuilder struct {
	limit  int
	offset int
}

func (o OffsetBuilder) Limit(limit int) OffsetBuilder {
	o.limit = limit
	return o
}

func (o OffsetBuilder) Offset(offset int) OffsetBuilder {
	o.offset = offset
	return o
}

func (o OffsetBuilder) Build() string {
	return fmt.Sprintf("LIMIT %d OFFSET %d", o.limit, o.offset)
}

func main() {
	o := OffsetBuilder{}
    fmt.Println(o.Limit(5).Offset(10).Build()) // LIMIT 5 OFFSET 10
	
	w := WhereBuilder{}
	fmt.Println(w.Where("id = 5").Where("status = 'ok'").Build()) // WHERE ID = 5 AND status = 'ok'
}

```

These builders are basic and get the job done, however, if you want to combine them to build more high level DSL, you will quickly realize the problem:

```go
package main

type SQLBuilder struct {
	WhereBuilder
	OffsetBuilder
}

func main() {
	s := SQLBuilder{}
	s.
		Where("id = 5"). // <-- returns WhereBuilder
		Limit()          // <-- defined in OffsetBuilder 
}
```

As you can see, golang builders cannot be efficiently combined, and current Generics implementation is not enough to achieve desired behavior.

## Solution

chaingen makes combination of different builders possible through code generation:

```go
// sql_builder.go
package main

type SQLBuilder struct {
	where WhereBuilder
	offset OffsetBuilder
}

func (s SQLBuilder) Build() string {
    return s.where.Build() + " " + s.offset.Build()
}

// sql_builder_chaingen.go: 
// code-generated methods

func (s SQLBuilder) Where(condition string) SQLBuilder {
	s.where = s.where.Where(condition)
	return s
}

func (s SQLBuilder) Limit(limit int) SQLBuilder {
	s.offset = s.offset.Limit(limit)
	return s
}

func (s SQLBuilder) Offset(offset int) SQLBuilder {
	s.offset = s.offset.Offset(offset)
	return s
}

```

chaingen will take notice of existing methods in the parent builder and skip their generation.

## Types of Methods

chaingen understands 2 different kinds of methods:

1. Chaining methods: `func (T) Method() T`
    * chaingen will wrap these methods and return parent builder type
2. Finalizer: `func (T) Method() <any other type>`
    * chaingen will proxy all these methods and return the original result type

## Usage

First, install chaingen binary:

```bash
$ go install github.com/AnatolyRugalev/chaingen@latest
```

Then, run chaingen providing package name and type of the final builder type:

```bash
$ chaingen -type SQLBuilder
```

### Options

You can alter chaingen behavior using these options:

```
Usage of chaingen:
  -build-tag string
        Sets go build tag name that is used to ignore generated files while analyzing code (default "chaingen")
  -err-on-conflict
        Whether to return error if method naming conflict is encountered (default true)
  -file-suffix string
        Generated file suffix, including '.go' (default ".chaingen.go")
  -recursive
        Whether to recuresively generate code for nested builders (default true)
  -src string
        Builder package directory (default "/home/anatoly/projects/AnatolyRugalev/chaingen")
  -struct-tag string
        Sets struct tag name to use (default "chaingen")
  -type string
        Builder struct type name. If not set, all struct types will be considered

```

### Go Generate

To generate a file using `go:generate`, add this line:

```go
//go:generate chaingen
```

Or if you don't want to keep `chaingen` in PATH:

```go
//go:generate github.com/AnatolyRugalev/chaingen/cmd/chaingen@latest
```

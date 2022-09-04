package offset

import (
	"fmt"
	"log"

	"golang.org/x/tools/go/packages"
)

type LimitBuilder struct {
	limit int
}

// Limit sets the limit
func (l LimitBuilder) Limit(limit int) LimitBuilder {
	l.limit = limit
	return l
}

// GetLimit returns the limit
func (l LimitBuilder) GetLimit() int {
	return l.limit
}

type OffsetBuilder struct {
	LimitBuilder
	offset int
}

// Offset sets the offset param
func (o OffsetBuilder) Offset(offset int) OffsetBuilder {
	o.offset = offset
	return o
}

func (o OffsetBuilder) GetOffest() int {
	return o.offset
}

// Build builds LIMIT+OFFSET SQL expression
func (o OffsetBuilder) Build() string {
	return fmt.Sprintf("LIMIT %d OFFSET %d", o.limit, o.offset)
}

// Finalizer is a useless finalizer that returns pointer to the builder
func (o OffsetBuilder) Finalizer() *OffsetBuilder {
	return &o
}

func (l LimitBuilder) ChainMethodWithExternalType(param1 *log.Logger, param2 packages.Module) LimitBuilder {
	return l
}

func (l LimitBuilder) FinalizerWithExternalType(param1 fmt.Formatter, param2 packages.Package) packages.Package {
	return param2
}

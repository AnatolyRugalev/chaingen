// Code generated by chaingen. DO NOT EDIT.
package sql_builder

import (
	"fmt"
	"github.com/AnatolyRugalev/chaingen/examples/sql_builder/offset"
	"golang.org/x/tools/go/packages"
	"log"
)

// Where sets SQL condition
func (s SQLBuilder) Where(condition string) SQLBuilder {
	s.WhereBuilder = s.WhereBuilder.Where(condition)
	return s
}

// Offset sets the offset param
func (s SQLBuilder) Offset(offset int) SQLBuilder {
	s.OffsetBuilder = s.OffsetBuilder.Offset(offset)
	return s
}

func (s SQLBuilder) GetOffest() int {
	return s.OffsetBuilder.GetOffest()
}

// Finalizer is a useless finalizer that returns pointer to the builder
func (s SQLBuilder) Finalizer() *offset.OffsetBuilder {
	return s.OffsetBuilder.Finalizer()
}

// Limit sets the limit
func (s SQLBuilder) Limit(limit int) SQLBuilder {
	s.OffsetBuilder = s.OffsetBuilder.Limit(limit)
	return s
}

// GetLimit returns the limit
func (s SQLBuilder) GetLimit() int {
	return s.OffsetBuilder.GetLimit()
}

func (s SQLBuilder) ChainMethodWithExternalType(param1 *log.Logger, param2 packages.Module) SQLBuilder {
	s.OffsetBuilder = s.OffsetBuilder.ChainMethodWithExternalType(param1, param2)
	return s
}

func (s SQLBuilder) FinalizerWithExternalType(param1 fmt.Formatter, param2 packages.Package) packages.Package {
	return s.OffsetBuilder.FinalizerWithExternalType(param1, param2)
}

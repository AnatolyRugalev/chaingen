// Code generated by chaingen. DO NOT EDIT.
package offset

import (
	"fmt"
	"golang.org/x/tools/go/packages"
	"log"
)

// Limit sets the limit
func (o OffsetBuilder) Limit(limit int) OffsetBuilder {
	o.LimitBuilder = o.LimitBuilder.Limit(limit)
	return o
}

// GetLimit returns the limit
func (o OffsetBuilder) GetLimit() int {
	return o.LimitBuilder.GetLimit()
}

func (o OffsetBuilder) ChainMethodWithExternalType(param1 *log.Logger, param2 packages.Module) OffsetBuilder {
	o.LimitBuilder = o.LimitBuilder.ChainMethodWithExternalType(param1, param2)
	return o
}

func (o OffsetBuilder) FinalizerWithExternalType(param1 fmt.Formatter, param2 packages.Package) packages.Package {
	return o.LimitBuilder.FinalizerWithExternalType(param1, param2)
}

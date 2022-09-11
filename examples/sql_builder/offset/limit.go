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
// This is a second line of the comment
func (l LimitBuilder) Limit(limit int) LimitBuilder {
	l.limit = limit
	return l
}

// GetLimit returns the limit
func (l LimitBuilder) GetLimit() int {
	return l.limit
}

func (l LimitBuilder) ChainMethodWithExternalType(param1 *log.Logger, param2 packages.Module) LimitBuilder {
	return l
}

func (l LimitBuilder) FinalizerWithExternalType(param1 fmt.Formatter, param2 packages.Package) packages.Package {
	return param2
}

func (l LimitBuilder) privateChainingMethod() LimitBuilder {
	return l
}

func (l LimitBuilder) VariadicMethod(params ...string) LimitBuilder {
	return l
}

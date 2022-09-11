package offset

import (
	"fmt"
)

type OffsetBuilder struct {
	L      LimitBuilder `chaingen:"*=Limit*,*Limit=*"`
	offset int
}

// Offset sets the offset param
func (o OffsetBuilder) Offset(offset int) OffsetBuilder {
	o.offset = offset
	return o
}

func (o OffsetBuilder) GetOffset() int {
	return o.offset
}

// Build builds LIMIT+OFFSET SQL expression
func (o OffsetBuilder) Build() string {
	return fmt.Sprintf("LIMIT %d OFFSET %d", o.L.limit, o.offset)
}

// Finalizer is a useless finalizer that returns pointer to the builder
func (o OffsetBuilder) Finalizer() *OffsetBuilder {
	return &o
}

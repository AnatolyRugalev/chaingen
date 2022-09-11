//go:generate go run github.com/AnatolyRugalev/chaingen

package sql_builder

import (
	"fmt"
	"strings"

	"github.com/AnatolyRugalev/chaingen/examples/sql_builder/offset"
)

type WhereBuilder struct {
	conditions []string
}

// Where sets SQL condition
func (w WhereBuilder) Where(condition string) WhereBuilder {
	w.conditions = append(w.conditions, condition)
	return w
}

func (w WhereBuilder) Build() string {
	return fmt.Sprintf("WHERE %s", strings.Join(w.conditions, " AND "))
}

type SQLBuilder struct {
	W WhereBuilder         `chaingen:"-Build,*=Where*,*Where=*"`
	O offset.OffsetBuilder `chaingen:"-Build,*=Offset*,*Offset=*"`
}

func (s SQLBuilder) Build() string {
	return s.W.Build() + " " + s.O.Build()
}

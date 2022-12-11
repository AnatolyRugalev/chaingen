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
	// TODO: auto-resolve method conflicts if parent builder already has it anyway
	W WhereBuilder         `chaingen:"-Build,*=Where*,*Where=*"`
	O offset.OffsetBuilder `chaingen:"-Build,Offset*=*,fin(GetLimit)=wrapper"`
}

func (s SQLBuilder) Build() string {
	return s.W.Build() + " " + s.O.Build()
}

func (s SQLBuilder) wrapper(params ...any) error {

	return nil
}

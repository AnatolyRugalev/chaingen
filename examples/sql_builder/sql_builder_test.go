package sql_builder

import "testing"

func TestSQLBuilder_Build(t *testing.T) {
	s := SQLBuilder{}
	sql := s.
		Where("id = 5").
		Limit(10).
		Offset(5).
		Build()
	if sql != "WHERE id = 5 LIMIT 10 OFFSET 5" {
		t.Fail()
	}
}

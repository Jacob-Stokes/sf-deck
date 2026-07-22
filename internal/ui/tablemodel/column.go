package tablemodel

import (
	"errors"
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var ErrUnknownColumn = errors.New("unknown column")

type Width struct {
	Min   int
	Ideal int
	Max   int
}

type ColumnDef[T any] struct {
	ID     string
	Header string
	Width  Width
	Style  lipgloss.Style

	Render  func(T) string
	SortKey func(T) string

	FetchFields []string
	Searchable  bool
	Exportable  bool
	Hidden      bool
	// Unsortable refuses column-sort on this column (composite/glyph
	// cells like the FLAGS strip). Propagated to the rendered
	// uilayout.ListColumn so the key handler can flash a hint.
	Unsortable bool
}

func (c ColumnDef[T]) Cell(row T) string {
	if c.Render == nil {
		return ""
	}
	return c.Render(row)
}

func (c ColumnDef[T]) SortCell(row T) string {
	if c.SortKey != nil {
		return c.SortKey(row)
	}
	return c.Cell(row)
}

func (c ColumnDef[T]) ListColumn() uilayout.ListColumn {
	header := c.Header
	if header == "" {
		header = c.ID
	}
	return uilayout.ListColumn{
		Name:       c.ID,
		Header:     header,
		Min:        c.Width.Min,
		Ideal:      c.Width.Ideal,
		Max:        c.Width.Max,
		Style:      c.Style,
		Unsortable: c.Unsortable,
	}
}

type Schema[T any] struct {
	Columns map[string]ColumnDef[T]

	DefaultColumns func(scope string) []string
	RequiredFields func(scope string) []string
	DynamicColumn  func(id string) (ColumnDef[T], bool)
}

func (s Schema[T]) Defaults(scope string) []string {
	if s.DefaultColumns == nil {
		return nil
	}
	return append([]string(nil), s.DefaultColumns(scope)...)
}

func (s Schema[T]) Required(scope string) []string {
	if s.RequiredFields == nil {
		return nil
	}
	return append([]string(nil), s.RequiredFields(scope)...)
}

func (s Schema[T]) Column(id string) (ColumnDef[T], bool) {
	if s.Columns != nil {
		if c, ok := s.Columns[id]; ok {
			return c, true
		}
	}
	if s.DynamicColumn != nil {
		return s.DynamicColumn(id)
	}
	return ColumnDef[T]{}, false
}

type UnknownColumnError struct {
	ID string
}

func (e UnknownColumnError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownColumn, e.ID)
}

func (e UnknownColumnError) Unwrap() error { return ErrUnknownColumn }

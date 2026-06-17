package ng

import (
	"errors"
	"fmt"
)

const defaultWidth = 3

// Part is a pre-resolved segment of the generated number.
// The caller populates Value from their domain model before calling Generate.
type Part struct {
	Position  int
	Separator string
	Value     string
}

// CounterConfig describes the auto-incrementing counter segment.
type CounterConfig struct {
	Position  int
	Separator string
	// Width is the zero-padded width of the counter (default 3).
	Width int
}

// Config describes the generated number format.
type Config struct {
	Counter CounterConfig
}

// GenerateParams are the inputs to Generator.Generate.
type GenerateParams struct {
	// Parts are the pre-resolved static segments of the number.
	Parts []Part

	// ContinuesIncrement enables counter sharing across parts at
	// WildcardPosition. The part at that position is replaced with '%' in
	// the SQL LIKE pattern so the counter is global across all values at
	// that position (e.g. shared increment across departments).
	ContinuesIncrement bool

	// WildcardPosition is the part position to wildcard when
	// ContinuesIncrement is true. Must match a position in Parts.
	WildcardPosition int

	Config Config
}

// GenerateResult is the output of Generator.Generate.
type GenerateResult struct {
	Number string
}

// Validate checks the configuration for logical consistency.
func (p *GenerateParams) Validate() error {
	if p.Config.Counter.Position <= 0 {
		return errors.New("counter position must be > 0")
	}

	positions := make(map[int]string)
	for i, part := range p.Parts {
		if part.Position <= 0 {
			return fmt.Errorf("part[%d]: position must be > 0", i)
		}
		if existing, dup := positions[part.Position]; dup {
			return fmt.Errorf("position %d is used by both %s and part[%d]", part.Position, existing, i)
		}
		positions[part.Position] = fmt.Sprintf("part[%d]", i)
	}

	for pos := range positions {
		if pos > p.Config.Counter.Position {
			return errors.New("counter must occupy the highest position")
		}
	}

	if p.ContinuesIncrement {
		if p.WildcardPosition <= 0 {
			return errors.New("wildcard position must be > 0 when continues increment is enabled")
		}
		found := false
		for _, part := range p.Parts {
			if part.Position == p.WildcardPosition {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("wildcard position %d not found in parts", p.WildcardPosition)
		}
	}

	return nil
}

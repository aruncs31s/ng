package ng

import (
	"errors"
	"fmt"
)

type PartConfig struct {
	Enabled   bool   `json:"enabled"`
	Position  int    `json:"position"`
	Separator string `json:"separator,omitempty"`
	Width     int    `json:"width,omitempty"`
	Length    int    `json:"length,omitempty"`
}

func (p *PartConfig) IsEnabled() bool { return p != nil && p.Enabled }

type PartConfigWithValue struct {
	PartConfig
	Value string
}

// ---------------------------------------------------------------------------
// Domain model types (mirrors the old model package)
// ---------------------------------------------------------------------------

type BatchInfo struct {
	BatchIdentifier           int
	BelongingCourseIdentifier int
	AcademicYearIdentifer     int
	StartYear                 string
}

type CourseInfo struct {
	Abbreviation         string
	DepartmentIdentifier int
}

type DepartmentInfo struct {
	Abbreviation string
}

type AcademicYearInfo struct {
	Name string
}

type RegCourseTypeInfo struct {
	Name string
	Code string
}

// ---------------------------------------------------------------------------
// Config types (mirrors the old AdmissionNumberConfig)
// ---------------------------------------------------------------------------

type AdmissionNumberConfig struct {
	CustomReferenceNumber *PartConfigWithValue
	Prefixes              []PartConfigWithValue
	University            *PartConfig
	Batch                 *PartConfig
	Year                  *PartConfig
	Department            *PartConfig
	RegType               *PartConfig
	IncrementalPart       *PartConfig
	UseCurrentYear        bool
}

func (c *AdmissionNumberConfig) Validate() error {
	if !c.IncrementalPart.IsEnabled() {
		return errors.New("incremental_part is required and must be enabled")
	}

	positions := make(map[int]string)
	maxPos := 0

	register := func(name string, cfg *PartConfig) error {
		if !cfg.IsEnabled() {
			return nil
		}
		if cfg.Position <= 0 {
			return fmt.Errorf("%s: position must be > 0", name)
		}
		if existing, dup := positions[cfg.Position]; dup {
			return fmt.Errorf("position %d is used by both %s and %s", cfg.Position, existing, name)
		}
		positions[cfg.Position] = name
		if cfg.Position > maxPos {
			maxPos = cfg.Position
		}
		return nil
	}

	configs := map[string]*PartConfig{
		"university":       c.University,
		"batch":            c.Batch,
		"year":             c.Year,
		"department":       c.Department,
		"reg_type":         c.RegType,
		"incremental_part": c.IncrementalPart,
	}
	if c.CustomReferenceNumber != nil {
		configs["custom-reference-number"] = &c.CustomReferenceNumber.PartConfig
	}
	for i := range c.Prefixes {
		name := fmt.Sprintf("prefixes[%d]", i)
		configs[name] = &c.Prefixes[i].PartConfig
	}

	for name, cfg := range configs {
		if err := register(name, cfg); err != nil {
			return err
		}
	}
	if c.IncrementalPart.Position != maxPos {
		return errors.New("incremental_part must occupy the last (highest) position")
	}
	return nil
}

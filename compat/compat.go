// Package compat provides a drop-in replacement for the v0 internal ERP
// generator API. It wraps the generic numbergenerator core and handles all
// domain-specific resolution (batch, course, department, etc.) internally.
//
// Usage (old API → new):
//
//	import "github.com/aruncs31s/ng/compat"
//
//	gen := compat.NewGenerator(myRepo, counterRepo)
//	result, err := gen.Generate(ctx, tx, compat.GenerateParams{...})
package compat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aruncs31s/ng"
	numbergenerator "github.com/aruncs31s/ng"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GenerateParams / GenerateResult
// ---------------------------------------------------------------------------

type GenerateParams struct {
	ContinuesIncrement bool
	BatchID            *int
	DepartmentNo       *int
	UniversityCode     *string
	RegTypeID          *int
	Config             ng.AdmissionNumberConfig
}

type GenerateResult struct {
	AdmissionNumber string
}

// ---------------------------------------------------------------------------
// Repository (domain lookups only — counters are handled by CounterRepository)
// ---------------------------------------------------------------------------

type Repository interface {
	// Batch is owned by a year and a course, and has the year info needed for prefix resolution.
	GetBatch(
		ctx context.Context,
		tx *gorm.DB,
		batchID int,
	) (ng.BatchInfo, error)
	GetCourse(
		ctx context.Context,
		tx *gorm.DB,
		courseID int,
	) (ng.CourseInfo, error)
	GetDepartment(
		ctx context.Context, tx *gorm.DB, departmentID int,
	) (ng.DepartmentInfo, error)
	GetAcademicYear(
		ctx context.Context,
		tx *gorm.DB, yearID int,
	) (ng.AcademicYearInfo, error)
	GetRegCourseType(
		ctx context.Context,
		tx *gorm.DB,
		regTypeID int,
	) (ng.RegCourseTypeInfo, error)
}

// ---------------------------------------------------------------------------
// Generator (drop-in replacement for the old service.Generator)
// ---------------------------------------------------------------------------

type Generator struct {
	repo Repository
	core *numbergenerator.Generator
}

func NewGenerator(repo Repository, counterRepo numbergenerator.CounterRepository) *Generator {
	return &Generator{
		repo: repo,
		core: numbergenerator.NewGenerator(counterRepo),
	}
}

func (g *Generator) Generate(ctx context.Context, tx *gorm.DB, p GenerateParams) (GenerateResult, error) {
	if g == nil {
		return GenerateResult{}, errors.New("generator not initialized")
	}
	if tx == nil {
		return GenerateResult{}, fmt.Errorf("transaction is nil")
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	if err := p.Config.Validate(); err != nil {
		return GenerateResult{}, fmt.Errorf("invalid config: %w", err)
	}

	// Resolve domain entities to string parts (same logic as old resolvePrefixParts).
	parts, wildcardPos, err := g.resolvePrefixParts(ctx, tx, p)
	if err != nil {
		return GenerateResult{}, err
	}

	coreResult, err := g.core.Generate(ctx, tx, numbergenerator.GenerateParams{
		Parts:              parts,
		ContinuesIncrement: p.ContinuesIncrement,
		WildcardPosition:   wildcardPos,
		Config: numbergenerator.Config{
			Counter: numbergenerator.CounterConfig{
				Position:  p.Config.IncrementalPart.Position,
				Separator: p.Config.IncrementalPart.Separator,
				Width:     p.Config.IncrementalPart.Width,
			},
		},
	})
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{AdmissionNumber: coreResult.Number}, nil
}

// ---------------------------------------------------------------------------
// Prefix resolution (same logic as the old service/generator.go)
// ---------------------------------------------------------------------------

func (g *Generator) resolvePrefixParts(
	ctx context.Context,
	tx *gorm.DB,
	p GenerateParams,
) ([]numbergenerator.Part, int, error) {
	c := p.Config
	parts := make([]numbergenerator.Part, 0, 5)

	var batch ng.BatchInfo
	needsBatch := c.Batch.IsEnabled() || c.Year.IsEnabled() || c.Department.IsEnabled()
	if needsBatch {
		if p.BatchID == nil {
			return nil, 0, errors.New("batchID is required when batch/year/department segment is enabled")
		}
		var err error
		batch, err = g.repo.GetBatch(ctx, tx, *p.BatchID)
		if err != nil {
			return nil, 0, fmt.Errorf("fetching batch: %w", err)
		}
	}

	if c.University.IsEnabled() {
		code := strings.ToUpper(strings.TrimSpace(strPtrOrEmpty(p.UniversityCode)))
		if code == "" {
			return nil, 0, errors.New("university code is required when university segment is enabled")
		}
		parts = append(parts, numbergenerator.Part{
			Position: c.University.Position, Separator: c.University.Separator, Value: code,
		})
	}

	if c.Batch.IsEnabled() {
		yearStr, err := g.resolveBatchYear(ctx, tx, batch, c.Batch, c.UseCurrentYear)
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, numbergenerator.Part{
			Position: c.Batch.Position, Separator: c.Batch.Separator, Value: yearStr,
		})
	}

	if c.Year.IsEnabled() {
		yearStr, err := g.resolveAcademicYear(ctx, tx, batch, c.Year, c.UseCurrentYear)
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, numbergenerator.Part{
			Position: c.Year.Position, Separator: c.Year.Separator, Value: yearStr,
		})
	}

	if c.Department.IsEnabled() {
		deptStr, err := g.resolveDepartment(ctx, tx, batch, p.DepartmentNo)
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, numbergenerator.Part{
			Position: c.Department.Position, Separator: c.Department.Separator, Value: deptStr,
		})
	}

	if c.RegType.IsEnabled() {
		regTypeStr, err := g.resolveRegType(ctx, tx, p.RegTypeID)
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, numbergenerator.Part{
			Position: c.RegType.Position, Separator: c.RegType.Separator, Value: regTypeStr,
		})
	}

	if c.CustomReferenceNumber != nil && c.CustomReferenceNumber.Enabled {
		val := strings.TrimSpace(c.CustomReferenceNumber.Value)
		if val != "" {
			parts = append(parts, numbergenerator.Part{
				Position:  c.CustomReferenceNumber.Position,
				Separator: c.CustomReferenceNumber.Separator,
				Value:     val,
			})
		}
	}

	for _, px := range c.Prefixes {
		if !px.Enabled {
			continue
		}
		val := strings.TrimSpace(px.Value)
		if val == "" {
			continue
		}
		parts = append(parts, numbergenerator.Part{
			Position: px.Position, Separator: px.Separator, Value: val,
		})
	}

	wildcardPos := 0
	if p.ContinuesIncrement && c.Department.IsEnabled() {
		wildcardPos = c.Department.Position
	}

	return parts, wildcardPos, nil
}

func (g *Generator) resolveBatchYear(ctx context.Context, tx *gorm.DB, batch ng.BatchInfo, cfg *ng.PartConfig, useCurrent bool) (string, error) {
	if useCurrent {
		return trimToLength(
			strconv.Itoa(
				time.Now().Year(),
			), cfg.Length), nil
	}
	if strings.TrimSpace(batch.StartYear) != "" {
		return trimToLength(batch.StartYear, cfg.Length), nil
	}
	if batch.AcademicYearIdentifer != 0 {
		ay, err := g.repo.GetAcademicYear(ctx, tx, batch.AcademicYearIdentifer)
		if err != nil {
			return "", err
		}
		return trimToLength(ay.Name, cfg.Length), nil
	}
	return "", errors.New("cannot resolve batch year: StartYear is empty and YearID is nil")
}

func (g *Generator) resolveAcademicYear(ctx context.Context, tx *gorm.DB, batch ng.BatchInfo, cfg *ng.PartConfig, useCurrent bool) (string, error) {
	if useCurrent {
		return trimToLength(strconv.Itoa(time.Now().Year()), cfg.Length), nil
	}
	if batch.AcademicYearIdentifer != 0 {
		ay, err := g.repo.GetAcademicYear(ctx, tx, batch.AcademicYearIdentifer)
		if err != nil {
			return "", err
		}
		return trimToLength(ay.Name, cfg.Length), nil
	}
	if strings.TrimSpace(batch.StartYear) != "" {
		return trimToLength(batch.StartYear, cfg.Length), nil
	}
	return "", errors.New("cannot resolve academic year: YearID is nil and StartYear is empty")
}

func (g *Generator) resolveDepartment(ctx context.Context, tx *gorm.DB, batch ng.BatchInfo, departmentNo *int) (string, error) {
	if batch.BelongingCourseIdentifier != 0 {
		course, err := g.repo.GetCourse(ctx, tx, batch.BelongingCourseIdentifier)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(course.Abbreviation) != "" {
			return strings.ToUpper(strings.TrimSpace(course.Abbreviation)), nil
		}
		if course.DepartmentIdentifier != 0 {
			departmentNo = &course.DepartmentIdentifier
		}
	}
	if departmentNo == nil {
		return "", errors.New("department is required when department segment is enabled")
	}
	// NOTE: passing batch.BatchIdentifier as departmentID is preserved from the
	// original code. If your schema maps department by batch, keep this;
	// otherwise update to *departmentNo.
	dept, err := g.repo.GetDepartment(ctx, tx, batch.BatchIdentifier)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dept.Abbreviation) == "" {
		return "", errors.New("department abbreviation is empty")
	}
	return strings.ToUpper(strings.TrimSpace(dept.Abbreviation)), nil
}

func (g *Generator) resolveRegType(ctx context.Context, tx *gorm.DB, regTypeID *int) (string, error) {
	if regTypeID == nil || *regTypeID <= 0 {
		return "", errors.New("regTypeID is required when reg_type segment is enabled")
	}
	rc, err := g.repo.GetRegCourseType(ctx, tx, *regTypeID)
	if err != nil {
		return "", fmt.Errorf("fetching reg course type: %w", err)
	}
	upperName := strings.ToUpper(rc.Name)
	if strings.Contains(upperName, "LET") || strings.Contains(upperName, "LATERAL ENTRY") {
		return "L", nil
	}
	trimmedCode := strings.TrimSpace(rc.Code)
	if trimmedCode != "" {
		return strings.ToUpper(trimmedCode[:1]), nil
	}
	trimmedName := strings.TrimSpace(rc.Name)
	if trimmedName != "" {
		return strings.ToUpper(trimmedName[:1]), nil
	}
	return "", errors.New("cannot resolve reg_type: name and code are empty")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func trimToLength(value string, length int) string {
	if length <= 0 {
		return value
	}
	if len(value) > length {
		return value[len(value)-length:]
	}
	return value
}

func strPtrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

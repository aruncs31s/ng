# number_generator

Race-safe, configurable sequential number generator. Assembles numbers from positional parts with one part being an auto-incrementing counter, using `SELECT FOR UPDATE` for concurrency safety.

The caller is responsible for resolving static prefix parts from their own domain model. Only the counter logic lives in this package.

## Install

```
go get github.com/aruncs31s/ng
```

## Dependencies

- [gorm.io/gorm](https://gorm.io) — the included repository implementation uses GORM

## Usage

### 1. Define your number format

The number is built from **parts** — each has a `Position`, `Value`, and optional `Separator`. One part is the auto-incrementing **counter**.

Example: admission number `26CSE001` (2-digit year + department code + 3-digit counter)

### 2. Set up the counter repository

```go
import (
    "github.com/aruncs31s/number_generator"
    "gorm.io/gorm"
)

repo := numbergenerator.NewGORMRepository(&numbergenerator.GORMConfig{
    TableName:       "users",           // your table holding the numbers
    ColumnName:      "user_name",       // your column holding the numbers
    CancelledPrefix: "",                // optional: prefix for cancelled entries
})
```

### 3. Create a generator

```go
gen := numbergenerator.NewGenerator(repo)
```

### 4. Resolve prefix parts and generate

```go
result, err := gen.Generate(ctx, tx, numbergenerator.GenerateParams{
    Parts: []numbergenerator.Part{
        {Position: 1, Value: "26"},   // year
        {Position: 2, Value: "CSE"},  // department code
    },
    Config: numbergenerator.Config{
        Counter: numbergenerator.CounterConfig{
            Position:  3,              // must be the highest position
            Separator: "",
            Width:     3,              // zero-padded, default 3
        },
    },
})

// result.Number == "26CSE001"
```

## Advanced: Shared counter across segments

When `ContinuesIncrement` is true, the counter is shared across all values at `WildcardPosition`. This is useful when a segment (e.g. department code) is embedded in the middle of the number and you want one global counter.

Example: numbers `26CSE001`, `26ECE002`, `26CSE003` — the counter is global across departments.

```go
result, err := gen.Generate(ctx, tx, numbergenerator.GenerateParams{
    Parts: []numbergenerator.Part{
        {Position: 1, Value: "26"},
        {Position: 2, Value: "CSE"},  // this segment is wildcarded
    },
    ContinuesIncrement: true,
    WildcardPosition:   2,            // position to wildcard
    Config: numbergenerator.Config{
        Counter: numbergenerator.CounterConfig{
            Position: 3,
            Width:    3,
        },
    },
})
```

The query uses `LIKE '26%CSE%'` (with the department replaced by `%`) to find the global max across all departments.

## Drop-in replacement for v0 (compat)

If you're migrating from the old internal ERP version that had domain resolution
(batch, course, department, reg type) built into the generator, use the `compat`
package. It wraps the core generator and handles all domain resolution
internally — your existing callers only change their imports.

```go
import (
    "github.com/aruncs31s/number_generator"
    "github.com/aruncs31s/number_generator/compat"
)

// Your existing domain repository (implements compat.Repository).
type myDomainRepo struct { ... }
func (r *myDomainRepo) GetBatch(ctx, tx, batchID) (compat.BatchInfo, error) { ... }
// ...

counterRepo := numbergenerator.NewGORMRepository(&numbergenerator.GORMConfig{
    TableName:  "users",
    ColumnName: "user_name",
})

gen := compat.NewGenerator(myDomainRepo, counterRepo)

result, err := gen.Generate(ctx, tx, compat.GenerateParams{
    BatchID:      &batchID,
    DepartmentNo: &deptNo,
    Config: compat.AdmissionNumberConfig{
        CustomReferenceNumber: &compat.PartConfigWithValue{
            PartConfig: compat.PartConfig{Enabled: true, Position: 1},
            Value:      "FAZ",
        },
        IncrementalPart: &compat.PartConfig{Enabled: true, Position: 2, Width: 3},
    },
})
// result.AdmissionNumber == "FAZ001"
```

### Migration at a glance

| Before (internal) | After (public + compat) |
|---|---|
| `service.NewGenerator(repo)` | `compat.NewGenerator(repo, counterRepo)` |
| `model.GenerateParams` | `compat.GenerateParams` |
| `model.GenerateResult` | `compat.GenerateResult` |
| `model.AdmissionNumberConfig` | `compat.AdmissionNumberConfig` |
| `model.PartConfig` | `compat.PartConfig` |
| `repository.Repository` | `compat.Repository` |

The `compat` package keeps the old domain types (`BatchInfo`, `CourseInfo`,
`DepartmentInfo`, `AcademicYearInfo`, `RegCourseTypeInfo`), the old config
validation (`AdmissionNumberConfig.Validate()`), and the old result shape
(`AdmissionNumber string`).

## Custom repository implementation

Implement `CounterRepository` for any backend:

```go
type CounterRepository interface {
    LockAndGetLastByPrefix(ctx, tx, prefix) (string, error)
    LockAndGetLastByPattern(ctx, tx, pattern) (string, error)
    CheckIfCancelled(ctx, tx, prefix, candidate) (bool, error)
    NumberExists(ctx, tx, number) (bool, error)
}
```

## Migration from v0 (internal ERP version)

**Recommended path:** use the `compat` package (see section above) — it's a
drop-in replacement requiring only import changes.

If you prefer to migrate to the raw core API, pre-resolve domain values into
`[]Part`:

```go
// Before (v0 — internal)
params := model.GenerateParams{
    BatchID:      &batchID,
    DepartmentNo: &deptNo,
    Config:       admissionNumberConfig,
}

// After (public core API)
params := numbergenerator.GenerateParams{
    Parts: []numbergenerator.Part{
        {Position: 1, Value: resolvedYear},
        {Position: 2, Value: resolvedDeptCode},
    },
    Config: numbergenerator.Config{
        Counter: numbergenerator.CounterConfig{Position: 3, Width: 3},
    },
}
```

## Validation

`GenerateParams.Validate()` checks:
- Counter position is > 0 and is the highest position
- All part positions are unique and > 0
- If `ContinuesIncrement`, `WildcardPosition` is set and exists in `Parts`

package ng

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CounterRepository handles atomic counter operations during number generation.
//
// Implementations must use SELECT FOR UPDATE (or equivalent) to serialise
// concurrent generation for the same prefix.
type CounterRepository interface {
	// LockAndGetLastByPrefix returns the value with the highest trailing
	// numeric value among all column values starting with the given prefix.
	LockAndGetLastByPrefix(ctx context.Context, tx *gorm.DB, prefix string) (string, error)

	// LockAndGetLastByPattern returns the value with the highest trailing
	// numeric value among all column values matching the given SQL LIKE pattern.
	// The pattern may contain embedded '%' wildcards.
	LockAndGetLastByPattern(ctx context.Context, tx *gorm.DB, pattern string) (string, error)

	// CheckIfCancelled returns true if the candidate number has a cancelled
	// entry in the store.
	CheckIfCancelled(ctx context.Context, tx *gorm.DB, prefix, candidate string) (bool, error)

	// NumberExists returns true if the given number already exists in the store.
	NumberExists(ctx context.Context, tx *gorm.DB, number string) (bool, error)
}

// GORMConfig configures a GORM-backed CounterRepository.
type GORMConfig struct {
	TableName  string
	ColumnName string
	// CancelledPrefix is the prefix prepended to mark cancelled numbers
	// (e.g. "CANCELED_"). If empty, cancelled-number features are disabled.
	CancelledPrefix string
}

type gormCounterRepo struct {
	c *GORMConfig
}

// NewGORMRepository returns a GORM-backed CounterRepository.
func NewGORMRepository(c *GORMConfig) CounterRepository {
	return &gormCounterRepo{c: c}
}

func (r *gormCounterRepo) LockAndGetLastByPrefix(ctx context.Context, tx *gorm.DB, prefix string) (string, error) {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if prefix == "" {
		return "", errors.New("prefix must not be empty")
	}
	var last string
	err := tx.WithContext(ctx).
		Table(r.c.TableName).
		Select(r.c.ColumnName).
		Where(r.c.ColumnName+" LIKE ?", prefix+"%").
		Order("LENGTH(" + r.c.ColumnName + ") DESC, " + r.c.ColumnName + " DESC").
		Limit(1).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Scan(&last).Error
	if err != nil {
		return "", err
	}
	return strings.ToUpper(strings.TrimSpace(last)), nil
}

func (r *gormCounterRepo) LockAndGetLastByPattern(ctx context.Context, tx *gorm.DB, pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", errors.New("pattern must not be empty")
	}
	var last string
	err := tx.WithContext(ctx).
		Table(r.c.TableName).
		Select(r.c.ColumnName).
		Where(r.c.ColumnName+" LIKE ?", pattern).
		Order("CAST(REGEXP_SUBSTR(" + r.c.ColumnName + ", '[0-9]+$') AS UNSIGNED) DESC").
		Limit(1).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Scan(&last).Error
	if err != nil {
		return "", err
	}
	return strings.ToUpper(strings.TrimSpace(last)), nil
}

func (r *gormCounterRepo) CheckIfCancelled(ctx context.Context, tx *gorm.DB, prefix, candidate string) (bool, error) {
	if r.c.CancelledPrefix == "" {
		return false, nil
	}
	var count int64
	err := tx.WithContext(ctx).
		Table(r.c.TableName).
		Where(r.c.ColumnName+" = ?", r.c.CancelledPrefix+candidate).
		Count(&count).Error
	return count > 0, err
}

func (r *gormCounterRepo) NumberExists(ctx context.Context, tx *gorm.DB, number string) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).
		Table(r.c.TableName).
		Where(r.c.ColumnName+" = ?", number).
		Count(&count).Error
	return count > 0, err
}

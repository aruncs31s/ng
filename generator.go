// Package numbergenerator provides a race-safe, configurable sequential number
// generator. It assembles numbers from positional parts with one part being an
// auto-incrementing counter, using SELECT FOR UPDATE for concurrency safety.
//
// The caller is responsible for resolving static prefix parts from their own
// domain model. Only the counter logic (find max, increment, insert) lives in
// this package.
package ng

import (
	"context"
	"fmt"
	"strconv"

	"gorm.io/gorm"
)

type Generator struct {
	counterRepo CounterRepository
}

func NewGenerator(counterRepo CounterRepository) *Generator {
	return &Generator{counterRepo: counterRepo}
}

func (g *Generator) Generate(ctx context.Context, tx *gorm.DB, p GenerateParams) (GenerateResult, error) {
	if g == nil {
		return GenerateResult{}, ErrGeneratorNotInitialized
	}
	if tx == nil {
		return GenerateResult{}, fmt.Errorf("transaction is nil")
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	if err := p.Validate(); err != nil {
		return GenerateResult{}, fmt.Errorf("invalid params: %w", err)
	}

	parts := make([]numberPart, len(p.Parts))
	for i, pt := range p.Parts {
		parts[i] = numberPart{position: pt.Position, separator: pt.Separator, value: pt.Value}
	}
	prefix := buildNumber(parts, true)

	var (
		number string
		err    error
	)
	if p.ContinuesIncrement && p.WildcardPosition > 0 {
		pattern := buildWildcardPattern(parts, p.WildcardPosition)
		number, err = g.generateNextGlobal(ctx, tx, parts, pattern, p.Config.Counter)
		if err != nil {
			return GenerateResult{}, err
		}
		return g.dedup(ctx, tx, number)
	}

	number, err = g.generateNext(ctx, tx, parts, prefix, p.Config.Counter)
	if err != nil {
		return GenerateResult{}, err
	}
	return g.dedup(ctx, tx, number)
}

func (g *Generator) dedup(ctx context.Context, tx *gorm.DB, number string) (GenerateResult, error) {
	exists, err := g.counterRepo.NumberExists(ctx, tx, number)
	if err != nil {
		return GenerateResult{}, err
	}
	if exists {
		return GenerateResult{}, fmt.Errorf("generated number %q already exists — possible race condition", number)
	}
	return GenerateResult{Number: number}, nil
}

func (g *Generator) generateNext(
	ctx context.Context,
	tx *gorm.DB,
	prefixParts []numberPart,
	prefix string,
	cfg CounterConfig,
) (string, error) {
	last, err := g.counterRepo.LockAndGetLastByPrefix(ctx, tx, prefix)
	if err != nil {
		return "", fmt.Errorf("locking last number: %w", err)
	}

	width := defaultWidth
	if cfg.Width > 0 {
		width = cfg.Width
	}

	nextNum := getNext(last, prefix)
	result := getNumber(nextNum, width, prefixParts, cfg)

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		yes, err := g.counterRepo.CheckIfCancelled(ctx, tx, prefix, result)
		if err != nil {
			return "", ErrCheckingCancelled
		}
		if !yes {
			break
		}
		nextNum++
		result = getNumber(nextNum, width, prefixParts, cfg)
	}
	return result, nil
}

func (g *Generator) generateNextGlobal(
	ctx context.Context,
	tx *gorm.DB,
	fullParts []numberPart,
	pattern string,
	cfg CounterConfig,
) (string, error) {
	last, err := g.counterRepo.LockAndGetLastByPattern(ctx, tx, pattern)
	if err != nil {
		return "", fmt.Errorf("locking last number (global pattern): %w", err)
	}

	width := defaultWidth
	if cfg.Width > 0 {
		width = cfg.Width
	}

	nextNum := getNext(last, "")
	numStr := strconv.Itoa(nextNum)
	if len(numStr) > width {
		width = len(numStr)
	}
	allParts := copyParts(fullParts, numberPart{
		position:  cfg.Position,
		separator: cfg.Separator,
		value:     fmt.Sprintf("%0*d", width, nextNum),
	})
	return buildNumber(allParts, false), nil
}

package ng

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockCounterRepo struct {
	lastByPrefix  map[string]string
	cancelledNums map[string]bool
	existingNums  map[string]bool
}

func (m *mockCounterRepo) LockAndGetLastByPrefix(ctx context.Context, tx *gorm.DB, prefix string) (string, error) {
	return m.lastByPrefix[prefix], nil
}

func (m *mockCounterRepo) LockAndGetLastByPattern(ctx context.Context, tx *gorm.DB, pattern string) (string, error) {
	return "", nil
}

func (m *mockCounterRepo) CheckIfCancelled(ctx context.Context, tx *gorm.DB, prefix, candidate string) (bool, error) {
	return m.cancelledNums[candidate], nil
}

func (m *mockCounterRepo) NumberExists(ctx context.Context, tx *gorm.DB, number string) (bool, error) {
	return m.existingNums[number], nil
}

func TestGeneratorAutoIncrement(t *testing.T) {
	// Setup in-memory sqlite to get a valid *gorm.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	repo := &mockCounterRepo{
		lastByPrefix: map[string]string{
			"26": "26137",
		},
		cancelledNums: map[string]bool{
			"26138": true, // Cancelled number, should be skipped
		},
		existingNums: map[string]bool{
			"26139": true, // Already exists in DB, should be skipped
		},
	}

	gen := NewGenerator(repo)

	params := GenerateParams{
		Parts: []Part{
			{Position: 1, Value: "26"},
		},
		Config: Config{
			Counter: CounterConfig{
				Position: 2,
				Width:    3,
			},
		},
	}

	res, err := gen.Generate(context.Background(), db, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 26137 was the last.
	// 26138 is cancelled -> skipped.
	// 26139 exists in DB -> skipped.
	// 26140 should be generated.
	expected := "26140"
	if res.Number != expected {
		t.Errorf("expected generated number %q, got %q", expected, res.Number)
	}
}

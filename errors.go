package ng

import "errors"

var (
	ErrGeneratorNotInitialized = errors.New("generator not initialized: call NewGenerator")

	ErrCheckingCancelled = errors.New("checking cancelled numbers")
)

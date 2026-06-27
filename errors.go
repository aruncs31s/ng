package ng

import "errors"

var (
	ErrGeneratorNotInitialized = errors.New("generator not initialized: call NewGenerator")

	ErrCheckingCancelled           = errors.New("checking cancelled numbers")
	ErrLockingLastNumber           = errors.New("error locking last number")
	ErrValidation                  = errors.New("error validation")
	ErrGeneratedNumberAlreadyExist = errors.New("generated number already exist")
)

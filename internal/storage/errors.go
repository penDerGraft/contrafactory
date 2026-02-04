package storage

import "errors"

// Common storage errors
var (
	ErrNotFound      = errors.New("not found")
	ErrVersionExists = errors.New("version already exists")
	ErrImmutable     = errors.New("version is immutable")
)

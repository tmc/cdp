package recorder

import (
	"errors"
	"fmt"
)

var (
	ErrNetworkRecord = errors.New("network record error")
	ErrValidation    = errors.New("validation error")
)

func wrapError(kind error, msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", kind, msg)
	}
	return fmt.Errorf("%w: %s: %w", kind, msg, err)
}

func fileError(kind error, op, path string, err error) error {
	return fmt.Errorf("%w: %s %q: %w", kind, op, path, err)
}

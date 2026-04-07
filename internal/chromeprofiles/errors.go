package chromeprofiles

import (
	"errors"
	"fmt"
)

var (
	ErrProfileSetup    = errors.New("profile setup error")
	ErrProfileNotFound = errors.New("profile not found")
	ErrProfileCopy     = errors.New("profile copy error")
	ErrConfig          = errors.New("configuration error")
)

func newError(kind error, msg string) error {
	return fmt.Errorf("%w: %s", kind, msg)
}

func wrapError(kind error, msg string, err error) error {
	if err == nil {
		return newError(kind, msg)
	}
	return fmt.Errorf("%w: %s: %w", kind, msg, err)
}

func fileError(kind error, op, path string, err error) error {
	return fmt.Errorf("%w: %s %q: %w", kind, op, path, err)
}

func withField(err error, key string, value any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (%s=%v)", err, key, value)
}

func profileSetup(msg string) error {
	return newError(ErrProfileSetup, msg)
}

func wrapProfileSetup(err error, msg string) error {
	return wrapError(ErrProfileSetup, msg, err)
}

func profileNotFound(msg string) error {
	return newError(ErrProfileNotFound, msg)
}

func wrapProfileNotFound(err error, msg string) error {
	return wrapError(ErrProfileNotFound, msg, err)
}

func profileCopy(msg string, err error) error {
	return wrapError(ErrProfileCopy, msg, err)
}

func configurationError(msg string) error {
	return newError(ErrConfig, msg)
}

func fileOpError(op, path string, err error) error {
	kind := ErrProfileCopy
	if op == "read" {
		kind = ErrProfileSetup
	}
	return fileError(kind, op, path, err)
}

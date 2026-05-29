package browser

import (
	"errors"
	"fmt"
)

var (
	ErrConnection  = errors.New("chrome connection error")
	ErrNavigation  = errors.New("chrome navigation error")
	ErrScript      = errors.New("chrome script error")
	ErrTimeout     = errors.New("chrome timeout")
	ErrNetwork     = errors.New("network error")
	ErrNetworkIdle = errors.New("network idle error")
	ErrNotLaunched = errors.New("browser not launched")
)

func notLaunchedError() error {
	return fmt.Errorf("%w: call Launch() first", ErrNotLaunched)
}

func withField(err error, key string, value any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (%s=%v)", err, key, value)
}

func navigationError(msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrNavigation, msg)
	}
	return fmt.Errorf("%w: %s: %w", ErrNavigation, msg, err)
}

func scriptError(msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrScript, msg)
	}
	return fmt.Errorf("%w: %s: %w", ErrScript, msg, err)
}

func timeoutError(msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrTimeout, msg)
	}
	return fmt.Errorf("%w: %s: %w", ErrTimeout, msg, err)
}

func networkError(msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrNetwork, msg)
	}
	return fmt.Errorf("%w: %s: %w", ErrNetwork, msg, err)
}

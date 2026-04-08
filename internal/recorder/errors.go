package recorder

import "errors"

var (
	ErrNetworkRecord = errors.New("network record error")
	ErrValidation    = errors.New("validation error")
)

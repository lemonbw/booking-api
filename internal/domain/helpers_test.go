package domain_test

import "errors"

func errorIs(err, target error) bool { return errors.Is(err, target) }

func errorAs(err error, target any) bool { return errors.As(err, target) }

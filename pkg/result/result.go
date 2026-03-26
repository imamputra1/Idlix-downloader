// Package result provides a generic container for handling success and failure
package result

// Result is a generic container that encapsulates a successful value or an error.
type Result[T any] struct {
	value T
	err   error
}

// Ok constructs a successful Result containing the provided value.
func Ok[T any](v T) Result[T] {
	return Result[T]{
		value: v,
		err:   nil,
	}
}

// Err constructs a failed Result containing the provided error.
func Err[T any](e error) Result[T] {
	var zero T
	return Result[T]{
		value: zero,
		err:   e,
	}
}

// Unwrap extracts the inner value and error as a standard Go tuple (T, error).
func (r Result[T]) Unwrap() (T, error) {
	return r.value, r.err
}

// IsOk returns true if the Result represents a success (no error).
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// IsErr returns true if the Result represents a failure (contains an error).
func (r Result[T]) isErr() bool {
	return r.err != nil
}

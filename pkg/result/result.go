package result

type Result[T any] struct {
	value T
	err   error
}

func Ok[T any](v T) Result[T] {
	return Result[T]{
		value: v,
		err:   nil,
	}
}

func Err[T any](e error) Result[T] {
	var zero T
	return Result[T]{
		value: zero,
		err:   e,
	}
}

func (r Result[T]) Unwrap() (T, error) {
	return r.value, r.err
}

func (r Result[T]) IsOk() bool {
	return r.err == nil
}

func (r Result[T]) IsErr() bool {
	return r.err != nil
}

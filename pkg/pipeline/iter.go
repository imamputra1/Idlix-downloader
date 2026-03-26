// Package pipeline provides generic functional utilities for slice manipulation
package pipeline

func Filter[T any](input []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range input {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

func Map[T any, R any](input []T, transform func(T) R) []R {
	result := make([]R, 0, len(input))
	for _, v := range input {
		result = append(result, transform(v))
	}
	return result
}

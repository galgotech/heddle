package runtime

import "heddle/pkg/runtime/interaction"

type Match[T ~string, V any] = interaction.Match[T, V]

func NewMatch[T ~string, V any](tag T, val V) Match[T, V] {
	return Match[T, V]{
		Tag: tag,
		Val: val,
	}
}

package sorted_set

import (
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

// SortedSet is an immutable view over a slice optimised for Contains checks, and sorted iteration.
type SortedSet[T comparable] struct {
	set   map[T]struct{}
	slice []T
}

// NewSortedSet makes a SortedSet of the elements in the passed slice.
// The original slice is not modified.
func NewSortedSet[T constraints.Ordered](in []T) *SortedSet[T] {
	ss := newSortedSetWithoutSortingSlice(in)
	slices.Sort(ss.slice)
	return ss
}

// NewSortedSetFunc makes a SortedSet of the elements in the passed slice, sorting by the passed func.
// The original slice is not modified.
func NewSortedSetFunc[T comparable](in []T, less func(a, b T) bool) *SortedSet[T] {
	ss := newSortedSetWithoutSortingSlice(in)
	slices.SortFunc(ss.slice, less)
	return ss
}

func newSortedSetWithoutSortingSlice[T comparable](in []T) *SortedSet[T] {
	set := make(map[T]struct{}, len(in))
	slice := make([]T, 0, len(in))
	for _, s := range in {
		if _, ok := set[s]; !ok {
			slice = append(slice, s)
		}
		set[s] = struct{}{}
	}
	return &SortedSet[T]{
		set:   set,
		slice: slice,
	}
}

// Contains returns whether the SortedSet contains the given string.
func (s *SortedSet[T]) Contains(v T) bool {
	if s == nil {
		return false
	}
	_, ok := s.set[v]
	return ok
}

// SortedSlice returns a sorted slice of the strings in the SortedSet.
// The returned slice must not be modified by the caller.
func (s *SortedSet[T]) SortedSlice() []T {
	if s == nil {
		return nil
	}
	return s.slice
}

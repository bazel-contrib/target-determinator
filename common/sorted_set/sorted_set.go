package sorted_set

import (
	"github.com/google/btree"
)

// SortedSet is a Set whose elements are traversable in sorted ordered.
type SortedSet[T any] struct {
	tree *btree.BTreeG[T]
}

const degree int = 2

// NewSortedSet makes a SortedSet of the elements in the passed slice.
// The original slice is not modified.
func NewSortedSet[T btree.Ordered](in []T) *SortedSet[T] {
	ss := &SortedSet[T]{
		tree: btree.NewOrderedG[T](degree),
	}
	for _, v := range in {
		ss.Add(v)
	}
	return ss
}

// NewSortedSetFn makes a SortedSet of the elements in the passed slice.
// The original slice is not modified.
func NewSortedSetFn[T any](in []T, less btree.LessFunc[T]) *SortedSet[T] {
	ss := &SortedSet[T]{
		tree: btree.NewG[T](degree, less),
	}
	for _, v := range in {
		ss.Add(v)
	}
	return ss
}

// Contains returns whether the SortedSet contains the given value.
func (s *SortedSet[T]) Contains(v T) bool {
	if s == nil {
		return false
	}
	return s.tree.Has(v)
}

// SortedSlice returns a sorted slice of the values in the SortedSet.
// The returned slice is owned by the caller, and may be modified freely.
func (s *SortedSet[T]) SortedSlice() []T {
	if s == nil {
		return nil
	}
	slice := make([]T, 0, s.tree.Len())
	s.tree.Ascend(func(v T) bool {
		slice = append(slice, v)
		return true
	})
	return slice
}

// Add adds an element to the SortedSet.
// If adding another SortedSet, AddAll is more efficient than repeated calls to
// Add as it avoids needing to materialize an intermediate slice.
func (s *SortedSet[T]) Add(v T) {
	s.tree.ReplaceOrInsert(v)
}

// AddAll adds all of the elements from the other SortedSet to this one.
func (s *SortedSet[T]) AddAll(other *SortedSet[T]) {
	other.tree.Ascend(func(v T) bool {
		s.Add(v)
		return true
	})
}

// Filter creates a new SortedSet containing only the elements from this one for
// which the passed predicate evaluates true.
func (s *SortedSet[T]) Filter(predicate func(T) bool) *SortedSet[T] {
	new := &SortedSet[T]{
		tree: s.tree.Clone(),
	}
	s.tree.Ascend(func(v T) bool {
		if !predicate(v) {
			new.tree.Delete(v)
		}
		return true
	})
	return new
}

// Clone makes a copy of this SortedSet.
// Behaviorally, the copy is completely independent of the original, but
// as the copying is done lazily with copy-on-write structures, performance
// of mutations on both sets may be worse while initial copies are (lazily)
// made.
func (s *SortedSet[T]) Clone() *SortedSet[T] {
	return &SortedSet[T]{
		tree: s.tree.Clone(),
	}
}

func (s *SortedSet[T]) Len() int {
	if s == nil {
		return 0
	}
	return s.tree.Len()
}

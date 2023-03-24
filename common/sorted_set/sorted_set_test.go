package sorted_set_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
	"github.com/google/uuid"
)

func TestContains(t *testing.T) {
	inSet := "cheese"
	notInSet := "onions"

	s := ss.NewSortedSet([]string{inSet})
	if !s.Contains(inSet) {
		t.Errorf("want contains %v got not", inSet)
	}
	if s.Contains(notInSet) {
		t.Errorf("want not contains %v got contains", notInSet)
	}
}

func TestSortedSlice(t *testing.T) {
	want := []string{"cheese", "onions"}
	{
		alreadySorted := ss.NewSortedSet([]string{"cheese", "onions"})
		got := alreadySorted.SortedSlice()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("already sorted: wanted %v got %v", want, got)
		}
	}

	{
		notAlreadySorted := ss.NewSortedSet([]string{"onions", "cheese"})
		got := notAlreadySorted.SortedSlice()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("not already sorted: wanted %v got %v", want, got)
		}
	}
}

func TestInsert(t *testing.T) {
	s := ss.NewSortedSet([]string{})

	{
		if len := s.Len(); len != 0 {
			t.Errorf("Want len 0 got %v", len)
		}

		got := s.SortedSlice()
		want := []string{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Before insert: wanted %v got %v")
		}

		if s.Contains("cheese") {
			t.Errorf("Before insert: wanted not contains cheese")
		}
		if s.Contains("onions") {
			t.Errorf("Before insert: wanted not contains onions")
		}
	}

	s.Add("cheese")
	{
		if len := s.Len(); len != 1 {
			t.Errorf("Want len 1 got %v", len)
		}

		got := s.SortedSlice()
		want := []string{"cheese"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("After one insert: wanted %v got %v")
		}

		if !s.Contains("cheese") {
			t.Errorf("After one insert: wanted contains cheese")
		}
		if s.Contains("onions") {
			t.Errorf("After one insert: wanted not contains onions")
		}
	}

	s.Add("cheese")
	{
		if len := s.Len(); len != 1 {
			t.Errorf("Want len 1 got %v", len)
		}

		got := s.SortedSlice()
		want := []string{"cheese"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("After two inserts: wanted %v got %v")
		}

		if !s.Contains("cheese") {
			t.Errorf("After two inserts: wanted contains cheese")
		}
		if s.Contains("onions") {
			t.Errorf("After two inserts: wanted not contains onions")
		}
	}

	s.Add("onions")
	{
		if len := s.Len(); len != 2 {
			t.Errorf("Want len 2 got %v", len)
		}

		got := s.SortedSlice()
		want := []string{"cheese", "onions"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("After three inserts: wanted %v got %v")
		}

		if !s.Contains("cheese") {
			t.Errorf("After three inserts: wanted contains cheese")
		}
		if !s.Contains("onions") {
			t.Errorf("After three inserts: wanted contains onions")
		}
	}

	s.Add("brie")
	{
		if len := s.Len(); len != 3 {
			t.Errorf("Want len 3 got %v", len)
		}

		got := s.SortedSlice()
		want := []string{"brie", "cheese", "onions"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("After four inserts: wanted %v got %v")
		}

		if !s.Contains("cheese") {
			t.Errorf("After four inserts: wanted contains cheese")
		}
		if !s.Contains("onions") {
			t.Errorf("After four inserts: wanted contains onions")
		}
		if !s.Contains("brie") {
			t.Errorf("After four inserts: wanted contains brie")
		}
	}
}

func TestRandomInsertsAlwaysSorted(t *testing.T) {
	s := ss.NewSortedSet([]string{})

	added := make(map[string]struct{})

	for len(added) < 1000000 {
		new := uuid.New().String()
		if _, ok := added[new]; !ok {
			added[new] = struct{}{}
			s.Add(new)
		}
	}
	sortedAdded := make([]string, 0, len(added))
	for a := range added {
		sortedAdded = append(sortedAdded, a)
	}
	sort.Strings(sortedAdded)

	if !reflect.DeepEqual(s.SortedSlice(), sortedAdded) {
		t.Errorf("Random additions weren't sorted")
	}
}

func TestNil(t *testing.T) {
	var set *ss.SortedSet[string]

	sortedSlice := set.SortedSlice()
	if len(sortedSlice) != 0 {
		t.Errorf("want nil SortedSlice to be empty but was: %v", sortedSlice)
	}

	if set.Contains("onions") {
		t.Errorf("want nil SortedSet to not contain anything but it did")
	}
}

func TestClone(t *testing.T) {
	s := ss.NewSortedSet([]string{"cheese", "onions"})
	s2 := s.Clone()

	s.Add("brie")
	s2.Add("cheddar")

	want1 := []string{"brie", "cheese", "onions"}
	got1 := s.SortedSlice()
	want2 := []string{"cheddar", "cheese", "onions"}
	got2 := s2.SortedSlice()
	if !reflect.DeepEqual(want1, got1) {
		t.Errorf("want %v got %v", want1, got1)
	}
	if !reflect.DeepEqual(want2, got2) {
		t.Errorf("want %v got %v", want2, got2)
	}
}

func TestCloneCustomComparator(t *testing.T) {
	s := ss.NewSortedSetFn([]string{"cheese", "onions"}, func(l, r string) bool {
		// Reverse order
		return l > r
	})
	s2 := s.Clone()

	{
		want1 := []string{"onions", "cheese"}
		got1 := s.SortedSlice()
		want2 := []string{"onions", "cheese"}
		got2 := s2.SortedSlice()
		if !reflect.DeepEqual(want1, got1) {
			t.Errorf("want %v got %v", want1, got1)
		}
		if !reflect.DeepEqual(want2, got2) {
			t.Errorf("want %v got %v", want2, got2)
		}
	}

	s.Add("brie")
	s2.Add("cheddar")

	{
		want1 := []string{"onions", "cheese", "brie"}
		got1 := s.SortedSlice()
		want2 := []string{"onions", "cheese", "cheddar"}
		got2 := s2.SortedSlice()
		if !reflect.DeepEqual(want1, got1) {
			t.Errorf("want %v got %v", want1, got1)
		}
		if !reflect.DeepEqual(want2, got2) {
			t.Errorf("want %v got %v", want2, got2)
		}
	}
}

func TestAddAll(t *testing.T) {
	s := ss.NewSortedSet([]string{"cheese", "onions"})
	s2 := ss.NewSortedSet([]string{"brie", "cheese", "cheddar"})

	s.AddAll(s2)

	want := []string{"brie", "cheddar", "cheese", "onions"}
	got := s.SortedSlice()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want %v got %v", want, got)
	}
}

func TestFilter(t *testing.T) {
	s := ss.NewSortedSet([]string{"brie", "cheese", "cheddar", "onions"})

	s2 := s.Filter(func(v string) bool {
		return strings.HasPrefix(v, "c")
	})

	want1 := []string{"brie", "cheddar", "cheese", "onions"}
	got1 := s.SortedSlice()
	if !reflect.DeepEqual(want1, got1) {
		t.Errorf("want %v got %v", want1, got1)
	}

	want2 := []string{"cheddar", "cheese"}
	got2 := s2.SortedSlice()
	if !reflect.DeepEqual(want2, got2) {
		t.Errorf("want %v got %v", want2, got2)
	}
}

func TestFilterCustomComparator(t *testing.T) {
	s := ss.NewSortedSetFn([]string{"brie", "cheese", "cheddar", "onions"}, func(l, r string) bool {
		// Reverse order
		return l > r
	})

	s2 := s.Filter(func(v string) bool {
		return strings.HasPrefix(v, "c")
	})

	want1 := []string{"onions", "cheese", "cheddar", "brie"}
	got1 := s.SortedSlice()
	if !reflect.DeepEqual(want1, got1) {
		t.Errorf("want %v got %v", want1, got1)
	}

	want2 := []string{"cheese", "cheddar"}
	got2 := s2.SortedSlice()
	if !reflect.DeepEqual(want2, got2) {
		t.Errorf("want %v got %v", want2, got2)
	}
}

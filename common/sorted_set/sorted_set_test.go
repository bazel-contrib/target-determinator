package sorted_set_test

import (
	"reflect"
	"testing"

	ss "github.com/bazel-contrib/target-determinator/common/sorted_set"
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

	type wrapper struct{ s string }
	wrappedLess := func(a, b wrapper) bool { return a.s < b.s }
	wantWrapped := []wrapper{{"cheese"}, {"onions"}}

	{
		fromFunc := ss.NewSortedSetFunc([]wrapper{{"cheese"}, {"onions"}}, wrappedLess)
		got := fromFunc.SortedSlice()
		if !reflect.DeepEqual(got, wantWrapped) {
			t.Errorf("already sorted: wanted %v got %v", wantWrapped, got)
		}
	}

	{
		fromFunc := ss.NewSortedSetFunc([]wrapper{{"onions"}, {"cheese"}}, wrappedLess)
		got := fromFunc.SortedSlice()
		if !reflect.DeepEqual(got, wantWrapped) {
			t.Errorf("already sorted: wanted %v got %v", wantWrapped, got)
		}
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

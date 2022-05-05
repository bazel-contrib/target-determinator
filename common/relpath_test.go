package common

import (
	"testing"
)

func TestRelPath_String(t *testing.T) {
	tests := []struct {
		name    string
		relPath RelPath
		want    string
	}{
		{
			name:    "Simple relative path",
			relPath: NewRelPath("foo/bar"),
			want:    "foo/bar",
		},
		{
			name:    "Simple relative path with dot-slash",
			relPath: NewRelPath("./foo/bar"),
			want:    "./foo/bar",
		},
		{
			name:    "Absolute path is treated as if they did not contain leading slashes",
			relPath: NewRelPath("/foo/bar"),
			want:    "foo/bar",
		},
		{
			name:    "Multiple leading slashes are handled correctly",
			relPath: NewRelPath("////foo/bar"),
			want:    "foo/bar",
		},
		{
			name:    "Empty paths return an empty string",
			relPath: NewRelPath(""),
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.relPath
			if got := r.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

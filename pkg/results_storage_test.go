package pkg

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
)

func TestParseS3URL(t *testing.T) {
	cases := []struct {
		in         string
		bucket     string
		prefix     string
		shouldFail bool
	}{
		{"s3://my-bucket", "my-bucket", "", false},
		{"s3://my-bucket/", "my-bucket", "", false},
		{"s3://my-bucket/some/prefix", "my-bucket", "some/prefix", false},
		{"s3://my-bucket/some/prefix/", "my-bucket", "some/prefix/", false},
		{"https://my-bucket", "", "", true},
		{"s3:///no-bucket", "", "", true},
		{"not a url", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			bucket, prefix, err := parseS3URL(tc.in)
			if tc.shouldFail {
				if err == nil {
					t.Fatalf("expected parse error for %q, got bucket=%q prefix=%q", tc.in, bucket, prefix)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if bucket != tc.bucket || prefix != tc.prefix {
				t.Fatalf("got (%q, %q), want (%q, %q)", bucket, prefix, tc.bucket, tc.prefix)
			}
		})
	}
}

// fakeS3 is an in-memory s3API used to exercise s3ResultsStorage without touching the network.
type fakeS3 struct {
	objects map[string][]byte
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := *in.Bucket + "/" + *in.Key
	data, ok := f.objects[key]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "NoSuchKey", Message: "not found"}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[*in.Bucket+"/"+*in.Key] = data
	return &s3.PutObjectOutput{}, nil
}

func TestS3ResultsStorageRoundTrip(t *testing.T) {
	fake := &fakeS3{}
	st := &s3ResultsStorage{bucket: "b", prefix: "pre", client: fake}

	if _, err := st.Load("k"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("expected cache miss, got %v", err)
	}

	if err := st.Save("k", []byte("hello")); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, ok := fake.objects["b/pre/k"]; !ok {
		t.Fatalf("expected object at b/pre/k, got %v", fake.objects)
	}

	data, err := st.Load("k")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", data, "hello")
	}
}

func TestResultsStorageForDispatch(t *testing.T) {
	t.Run("nil when neither is set", func(t *testing.T) {
		s, err := resultsStorageFor(&Context{})
		if err != nil || s != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", s, err)
		}
	})

	t.Run("local when only CacheDirectory is set", func(t *testing.T) {
		s, err := resultsStorageFor(&Context{CacheDirectory: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if _, ok := s.(*localResultsStorage); !ok {
			t.Fatalf("got %T, want *localResultsStorage", s)
		}
	})

	t.Run("malformed S3CacheURL fails", func(t *testing.T) {
		if _, err := resultsStorageFor(&Context{S3CacheURL: "not-a-url"}); err == nil {
			t.Fatal("expected error for malformed S3 URL")
		}
	})
}

func TestS3ResultsStorageNoPrefix(t *testing.T) {
	fake := &fakeS3{}
	st := &s3ResultsStorage{bucket: "b", client: fake}
	if err := st.Save("k", []byte("x")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := fake.objects["b/k"]; !ok {
		t.Fatalf("expected object at b/k, got %v", fake.objects)
	}
}

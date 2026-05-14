package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// ErrCacheMiss is returned by ResultsStorage.Load when no cache entry exists for the key.
var ErrCacheMiss = errors.New("cache miss")

// ResultsStorage abstracts how serialized cache entries are persisted.
type ResultsStorage interface {
	Load(key string) ([]byte, error)
	Save(key string, data []byte) error
	// Describe returns a human-readable location for log messages (e.g. a path or URL).
	Describe(key string) string
}

// resultsStorageFor returns the storage backend for the configured results cache, or nil if
// no results-cache backend is configured. S3CacheURL, when set, wins over CacheDirectory for
// results; CacheDirectory is independently used for the local git-worktree cache regardless.
func resultsStorageFor(ctx *Context) (ResultsStorage, error) {
	if ctx.S3CacheURL != "" {
		return newS3ResultsStorage(ctx.S3CacheURL)
	}
	if ctx.CacheDirectory != "" {
		return &localResultsStorage{baseDir: filepath.Join(ctx.CacheDirectory, configuredTargetCacheDirname)}, nil
	}
	return nil, nil
}

// localResultsStorage stores cache entries as files under a local directory.
type localResultsStorage struct {
	baseDir string
}

func (s *localResultsStorage) Describe(key string) string {
	return filepath.Join(s.baseDir, key)
}

func (s *localResultsStorage) Load(key string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.baseDir, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return data, nil
}

func (s *localResultsStorage) Save(key string, data []byte) error {
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir (%s): %w", s.baseDir, err)
	}
	finalPath := filepath.Join(s.baseDir, key)
	tmpFile, err := os.CreateTemp(s.baseDir, key+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp cache file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("failed to move temp cache file to final location: %w", err)
	}
	tmpPath = ""
	return nil
}

// s3ResultsStorage stores cache entries as S3 objects under a bucket and key prefix.
type s3ResultsStorage struct {
	bucket string
	prefix string
	client s3API
}

// s3API is the subset of the S3 client used here; defined as an interface to allow tests
// to substitute a fake.
type s3API interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func newS3ResultsStorage(rawURL string) (*s3ResultsStorage, error) {
	bucket, prefix, err := parseS3URL(rawURL)
	if err != nil {
		return nil, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &s3ResultsStorage{
		bucket: bucket,
		prefix: prefix,
		client: s3.NewFromConfig(cfg),
	}, nil
}

// ValidateS3CacheURL checks that rawURL is a well-formed s3://bucket[/prefix] URL.
// Intended for fail-fast validation at CLI flag parsing time.
func ValidateS3CacheURL(rawURL string) error {
	_, _, err := parseS3URL(rawURL)
	return err
}

// parseS3URL parses a URL of the form s3://bucket/optional/prefix into its bucket and prefix
// components. A trailing slash on the prefix is preserved as-is during key construction.
func parseS3URL(rawURL string) (bucket, prefix string, err error) {
	u, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid S3 URL %q: %w", rawURL, parseErr)
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("S3 URL must have scheme s3:// (got %q)", rawURL)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("S3 URL must include a bucket (got %q)", rawURL)
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}

func (s *s3ResultsStorage) objectKey(key string) string {
	if s.prefix == "" {
		return key
	}
	if strings.HasSuffix(s.prefix, "/") {
		return s.prefix + key
	}
	return s.prefix + "/" + key
}

func (s *s3ResultsStorage) Describe(key string) string {
	return fmt.Sprintf("s3://%s/%s", s.bucket, s.objectKey(key))
}

func (s *s3ResultsStorage) Load(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("S3 GetObject %s failed: %w", s.Describe(key), err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body %s: %w", s.Describe(key), err)
	}
	return data, nil
}

func (s *s3ResultsStorage) Save(key string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("S3 PutObject %s failed: %w", s.Describe(key), err)
	}
	return nil
}

// isS3NotFound returns true if err represents an S3 "object not found" condition.
// S3's GetObject returns NoSuchKey, but some compatible implementations return a 404
// wrapped as a generic API error.
func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "NoSuchKey" || code == "NotFound" {
			return true
		}
	}
	return false
}

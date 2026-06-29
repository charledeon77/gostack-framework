package storage

// Purpose: To provide an AWS S3-backed StorageDriver for Vault's production deployments.
// Philosophy: The Local disk driver is excellent for development and single-server deployments.
// But any horizontally-scaled or serverless deployment needs a shared, durable object store.
// By implementing the same contract.Storage interface, all application-level code (e.g.
// `storage.Put("avatars/user1.png", data)`) works identically whether files land on disk or S3.
// Architecture:
// Wraps the official AWS SDK v2 for Go. All S3 operations are context-aware. The S3Driver
// is initialized with a pre-configured `s3.Client`, bucket name, and an optional key prefix
// (similar to Laravel's `filesystem.disk.s3.root` config).
// Choice:
// We use AWS SDK v2 (not v1) as it is the current, actively maintained version with native
// context propagation and cleaner generics. It is also compatible with MinIO and any
// S3-compatible API (Cloudflare R2, Backblaze B2) via a custom endpoint override.
// Implementation:
// - Put: streams []byte as an io.Reader into s3.PutObject.
// - Get: downloads the S3 object body and reads it into a []byte.
// - Delete: issues s3.DeleteObject for the given key.
// - Exists: performs a HeadObject; no error means the file exists.

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Driver implements contract.Storage backed by AWS S3 (or any S3-compatible endpoint).
type S3Driver struct {
	client *s3.Client
	bucket string
	prefix string
	ctx    context.Context
}

// S3Config holds the configuration parameters for an S3 connection.
type S3Config struct {
	Region          string
	Bucket          string
	Prefix          string // Optional path prefix for all keys
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // Optional: set for MinIO / Cloudflare R2
}

// NewS3Driver creates a new S3Driver from the given configuration.
func NewS3Driver(cfg S3Config) (*S3Driver, error) {
	credProvider := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credProvider),
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3driver: failed to load AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		// Custom endpoint for MinIO / Cloudflare R2 / Backblaze B2.
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	return &S3Driver{
		client: s3.NewFromConfig(awsCfg, clientOpts...),
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
		ctx:    context.Background(),
	}, nil
}

func (s *S3Driver) key(path string) string {
	if s.prefix != "" {
		return s.prefix + "/" + path
	}
	return path
}

// Put uploads data bytes to S3 under the given path key.
func (s *S3Driver) Put(path string, contents []byte) error {
	_, err := s.client.PutObject(s.ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
		Body:   bytes.NewReader(contents),
	})
	if err != nil {
		return fmt.Errorf("s3driver: put failed for key '%s': %w", path, err)
	}
	return nil
}

// Get downloads the file at the given path from S3 and returns its bytes.
func (s *S3Driver) Get(path string) ([]byte, error) {
	result, err := s.client.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		return nil, fmt.Errorf("s3driver: get failed for key '%s': %w", path, err)
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

// Delete removes the file at the given path from S3.
func (s *S3Driver) Delete(path string) error {
	_, err := s.client.DeleteObject(s.ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		return fmt.Errorf("s3driver: delete failed for key '%s': %w", path, err)
	}
	return nil
}

// Exists returns true if the given S3 key exists, using a cheap HeadObject call.
func (s *S3Driver) Exists(path string) bool {
	_, err := s.client.HeadObject(s.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	return err == nil
}

package kms

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// Client wraps AWS KMS operations for envelope encryption.
type Client struct {
	kmsClient *kms.Client
	keyARN    string
}

// New creates a KMS client. If endpoint is non-empty, it overrides the
// default AWS endpoint (for LocalStack in dev).
func New(ctx context.Context, keyARN, region, endpoint string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("kms load config: %w", err)
	}

	kmsOpts := []func(*kms.Options){}
	if endpoint != "" {
		kmsOpts = append(kmsOpts, func(o *kms.Options) {
			o.BaseEndpoint = &endpoint
		})
	}

	return &Client{
		kmsClient: kms.NewFromConfig(cfg, kmsOpts...),
		keyARN:    keyARN,
	}, nil
}

// CreateKey provisions a new KMS key and returns its ARN.
// Used during tenant provisioning to create a per-tenant KEK.
func (c *Client) CreateKey(ctx context.Context, description string) (string, error) {
	out, err := c.kmsClient.CreateKey(ctx, &kms.CreateKeyInput{
		Description: &description,
		KeyUsage:    "ENCRYPT_DECRYPT",
		KeySpec:     "SYMMETRIC_DEFAULT",
	})
	if err != nil {
		return "", fmt.Errorf("kms create key: %w", err)
	}
	return *out.KeyMetadata.Arn, nil
}

// GenerateDataKey generates a new DEK using the client's default key ARN.
func (c *Client) GenerateDataKey(ctx context.Context) (plaintext, encrypted []byte, err error) {
	return c.GenerateDataKeyWithARN(ctx, c.keyARN)
}

// GenerateDataKeyWithARN generates a new DEK using a specific key ARN (per-tenant KEK).
func (c *Client) GenerateDataKeyWithARN(ctx context.Context, keyARN string) (plaintext, encrypted []byte, err error) {
	out, err := c.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   &keyARN,
		KeySpec: "AES_256",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("kms generate data key: %w", err)
	}
	return out.Plaintext, out.CiphertextBlob, nil
}

// WrapKey encrypts a DEK using the KMS KEK.
func (c *Client) WrapKey(ctx context.Context, dek []byte) ([]byte, error) {
	out, err := c.kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     &c.keyARN,
		Plaintext: dek,
	})
	if err != nil {
		return nil, fmt.Errorf("kms wrap key: %w", err)
	}
	return out.CiphertextBlob, nil
}

// UnwrapKey decrypts a KMS-encrypted DEK back to plaintext.
func (c *Client) UnwrapKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	out, err := c.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedDEK,
	})
	if err != nil {
		return nil, fmt.Errorf("kms unwrap key: %w", err)
	}
	return out.Plaintext, nil
}

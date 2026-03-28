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

// GenerateDataKey generates a new DEK and returns both plaintext and
// KMS-encrypted copies. Use the plaintext DEK for encryption, store
// the encrypted DEK alongside the ciphertext.
func (c *Client) GenerateDataKey(ctx context.Context) (plaintext, encrypted []byte, err error) {
	out, err := c.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   &c.keyARN,
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

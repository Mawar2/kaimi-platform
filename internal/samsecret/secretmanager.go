package samsecret

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// SecretManagerWriter is the production Writer: it adds a new version to a tenant's
// SAM.gov key secret in Google Secret Manager. The deployment's pipeline reads the
// same secret's "latest" version, so a saved key reaches the next hunt automatically.
//
// It authenticates via ADC (the runtime service account), which must hold
// roles/secretmanager.secretVersionAdder on the target secret — a narrow grant that
// allows adding versions but NOT reading existing ones, keeping the write path
// least-privilege.
type SecretManagerWriter struct {
	client *secretmanager.Client
	// parent is the fully-qualified secret resource name:
	// projects/{project}/secrets/{secretID}. AddSecretVersion appends under it.
	parent string
}

// NewSecretManagerWriter connects to Secret Manager via ADC and targets the given
// secret. projectID and secretID are required (the deployment's SAM secret). Caller
// owns Close.
func NewSecretManagerWriter(ctx context.Context, projectID, secretID string) (*SecretManagerWriter, error) {
	if projectID == "" || secretID == "" {
		return nil, fmt.Errorf("samsecret: writer requires a project id and secret id")
	}
	c, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("samsecret: secret manager client: %w", err)
	}
	return &SecretManagerWriter{
		client: c,
		parent: fmt.Sprintf("projects/%s/secrets/%s", projectID, secretID),
	}, nil
}

// Close releases the Secret Manager client.
func (w *SecretManagerWriter) Close() error { return w.client.Close() }

// Save validates the key and adds it as a new secret version (which becomes the new
// "latest"). It never logs the key. The bytes are the raw key with surrounding
// whitespace already rejected by ValidateKey, so no trimming alters what is stored.
func (w *SecretManagerWriter) Save(ctx context.Context, apiKey string) error {
	if err := ValidateKey(apiKey); err != nil {
		return err
	}
	_, err := w.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  w.parent,
		Payload: &secretmanagerpb.SecretPayload{Data: []byte(apiKey)},
	})
	if err != nil {
		// Do not include apiKey in the error; only the operation context.
		return fmt.Errorf("samsecret: add secret version: %w", err)
	}
	return nil
}

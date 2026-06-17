package productkey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// firestoreCollection holds one document per product key, keyed by the canonical
// key string. A separate "BlueMeta licensing" data store (a dedicated GCP
// project's Firestore) keeps key metadata isolated from any customer data.
const firestoreCollection = "product_keys"

// FirestoreRegistry is the production Registry: keys live in Firestore so an
// operator can revoke instantly and the gate validates with a single document
// read. It authenticates via ADC (the runtime service account), matching the
// rest of the deployment — no exported keys.
type FirestoreRegistry struct {
	client *firestore.Client
}

// NewFirestoreRegistry connects to the project's Firestore via ADC. Caller owns
// Close. projectID is required (the licensing project).
func NewFirestoreRegistry(ctx context.Context, projectID string) (*FirestoreRegistry, error) {
	if projectID == "" {
		return nil, errors.New("productkey: Firestore registry requires a GCP project id")
	}
	c, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("productkey: firestore client: %w", err)
	}
	return &FirestoreRegistry{client: c}, nil
}

// Close releases the Firestore client.
func (r *FirestoreRegistry) Close() error { return r.client.Close() }

// Mint generates a unique key for tester valid for ttl and Creates its document.
// Create (not Set) fails if the id already exists, so the generate-and-Create
// loop guarantees uniqueness even against a concurrent mint.
func (r *FirestoreRegistry) Mint(ctx context.Context, tester string, ttl time.Duration) (Record, error) {
	now := time.Now().UTC()
	for range 5 {
		key, err := GenerateKey()
		if err != nil {
			return Record{}, err
		}
		rec := Record{
			Key:       key,
			Tester:    tester,
			IssuedAt:  now,
			ExpiresAt: now.Add(ttl),
			Revoked:   false,
		}
		_, err = r.client.Collection(firestoreCollection).Doc(key).Create(ctx, rec)
		if err == nil {
			return rec, nil
		}
		if status.Code(err) == codes.AlreadyExists {
			continue // astronomically unlikely collision; try another key
		}
		return Record{}, fmt.Errorf("productkey: mint: %w", err)
	}
	return Record{}, errors.New("productkey: mint: could not generate a unique key")
}

// Lookup returns the record for key (after Normalize) or ErrNotFound.
func (r *FirestoreRegistry) Lookup(ctx context.Context, key string) (Record, error) {
	doc, err := r.client.Collection(firestoreCollection).Doc(Normalize(key)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("productkey: lookup: %w", err)
	}
	var rec Record
	if err := doc.DataTo(&rec); err != nil {
		return Record{}, fmt.Errorf("productkey: decode record: %w", err)
	}
	return rec, nil
}

// Revoke flips revoked=true, or returns ErrNotFound for an unknown key.
func (r *FirestoreRegistry) Revoke(ctx context.Context, key string) error {
	_, err := r.client.Collection(firestoreCollection).Doc(Normalize(key)).
		Update(ctx, []firestore.Update{{Path: "revoked", Value: true}})
	if status.Code(err) == codes.NotFound {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("productkey: revoke: %w", err)
	}
	return nil
}

// List returns every persisted record.
func (r *FirestoreRegistry) List(ctx context.Context) ([]Record, error) {
	var out []Record
	iter := r.client.Collection(firestoreCollection).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("productkey: list: %w", err)
		}
		var rec Record
		if err := doc.DataTo(&rec); err != nil {
			return nil, fmt.Errorf("productkey: decode record: %w", err)
		}
		out = append(out, rec)
	}
	return out, nil
}

package domain

import "context"

// VectorStore defines the interface for interacting with a vector database.
type VectorStore interface {
	// Upsert adds or updates snippets in the vector store.
	Upsert(ctx context.Context, snippets []Snippet) error
	// Query searches for snippets similar to the given text.
	Query(ctx context.Context, embedding Embedding, k int) ([]Snippet, error)
}

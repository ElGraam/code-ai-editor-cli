package domain

import "context"

// Embedding represents a numerical vector representation of text.
type Embedding []float32

// EmbeddingClient defines the interface for generating embeddings from text.
type EmbeddingClient interface {
	// GenerateEmbeddings generates embeddings for the given texts.
	GenerateEmbeddings(ctx context.Context, texts []string) ([]Embedding, error)
}

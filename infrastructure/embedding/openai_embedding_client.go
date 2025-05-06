package embedding

import (
	"context"
	"errors"
	"os"

	"code-ai-editor/domain"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIEmbeddingClient implements the domain.EmbeddingClient interface using the OpenAI API.
type OpenAIEmbeddingClient struct {
	client *openai.Client
	model  openai.EmbeddingModel // e.g., text-embedding-3-small
}

// NewOpenAIEmbeddingClient creates a new OpenAIEmbeddingClient.
// It reads the API key from the OPENAI_API_KEY environment variable.
func NewOpenAIEmbeddingClient(model openai.EmbeddingModel) (*OpenAIEmbeddingClient, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY environment variable not set")
	}
	client := openai.NewClient(apiKey)
	return &OpenAIEmbeddingClient{client: client, model: model}, nil
}

// GenerateEmbeddings generates embeddings for the given texts using the specified OpenAI model.
func (c *OpenAIEmbeddingClient) GenerateEmbeddings(ctx context.Context, texts []string) ([]domain.Embedding, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	req := openai.EmbeddingRequest{
		Input: texts,
		Model: c.model,
	}

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, err
	}

	embeddings := make([]domain.Embedding, len(resp.Data))
	for i, data := range resp.Data {
		// Assuming the embedding is []float32, adjust if needed based on the library version
		embeddings[i] = domain.Embedding(data.Embedding)
	}

	return embeddings, nil
}

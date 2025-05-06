package vectorstore

import (
	"context"
	"fmt"
	"log"
	"os"

	"code-ai-editor/domain"

	"github.com/google/uuid"
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// QdrantClient implements the domain.VectorStore interface using Qdrant.
type QdrantClient struct {
	client         qdrant.PointsClient
	collectionName string
}

// NewQdrantClient creates a new QdrantClient.
// It reads the Qdrant address and collection name from environment variables.
func NewQdrantClient() (*QdrantClient, error) {
	qdrantAddr := os.Getenv("QDRANT_ADDR")
	if qdrantAddr == "" {
		// Use default address if environment variable is not set
		qdrantAddr = "localhost:6334"
		log.Printf("QDRANT_ADDR environment variable not set, using default: %s\n", qdrantAddr)
	}
	collectionName := os.Getenv("QDRANT_COLLECTION_NAME")
	if collectionName == "" {
		collectionName = "code_snippets" // Default collection name
	}

	conn, err := grpc.NewClient(qdrantAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("could not connect to Qdrant: %w", err)
	}

	// Create PointsClient for operations on points (vectors)
	pointsClient := qdrant.NewPointsClient(conn)

	// Create CollectionsClient for collection management
	collectionsClient := qdrant.NewCollectionsClient(conn)

	client := &QdrantClient{
		client:         pointsClient,
		collectionName: collectionName,
	}

	// Ensure collection exists
	err = client.ensureCollectionExists(context.Background(), collectionsClient)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure collection exists: %w", err)
	}

	return client, nil
}

// ensureCollectionExists checks if the collection exists and creates it if it doesn't.
func (c *QdrantClient) ensureCollectionExists(ctx context.Context, collectionsClient qdrant.CollectionsClient) error {
	// Check if collection exists
	_, err := collectionsClient.Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: c.collectionName,
	})

	if err != nil {
		// Collection doesn't exist, create it
		log.Printf("Collection %s does not exist, creating...\n", c.collectionName)

		// Create collection with default settings for embeddings
		// Using size 1536 for OpenAI embeddings (or adjust based on your model)
		vectorSize := uint64(1536)

		_, err = collectionsClient.Create(ctx, &qdrant.CreateCollection{
			CollectionName: c.collectionName,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     vectorSize,
				Distance: qdrant.Distance_Cosine,
			}),
		})

		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}

		log.Printf("Collection %s created successfully\n", c.collectionName)
	}

	return nil
}

// Helper function to convert interface{} map to map[string]*qdrant.Value
func mapToPayload(data map[string]interface{}) (map[string]*qdrant.Value, error) {
	payload := make(map[string]*qdrant.Value)
	for key, val := range data {
		switch v := val.(type) {
		case string:
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
		case int:
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
		case int64:
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: v}}
		case float64:
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: v}}
		case bool:
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: v}}
		case []string: // Handle string slices specifically for 'symbols'
			listValues := make([]*qdrant.Value, len(v))
			for i, s := range v {
				listValues[i] = &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: s}}
			}
			payload[key] = &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: &qdrant.ListValue{Values: listValues}}}
		default:
			// Handle other types or return an error if unsupported
			return nil, fmt.Errorf("unsupported type for payload field '%s': %T", key, v)
		}
	}
	return payload, nil
}

// Upsert adds or updates snippets in the Qdrant collection.
func (c *QdrantClient) Upsert(ctx context.Context, snippets []domain.Snippet) error {
	if len(snippets) == 0 {
		return nil
	}

	points := make([]*qdrant.PointStruct, 0, len(snippets))
	for _, s := range snippets {
		if s.Embedding == nil {
			continue
		}

		pointID := s.ID
		if pointID == "" {
			u, err := uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("failed to generate UUID: %w", err)
			}
			pointID = u.String()
		}

		// Convert snippet metadata to Qdrant payload
		payloadMap := map[string]interface{}{
			"content":    s.Content,
			"file_path":  s.FilePath,
			"start_line": s.StartLine,
			"end_line":   s.EndLine,
			"symbols":    s.Symbols,
		}

		// Add custom metadata fields if they exist
		if s.Metadata != nil && len(s.Metadata) > 0 {
			for k, v := range s.Metadata {
				payloadMap[k] = v
			}
		}

		qdrantPayload, err := mapToPayload(payloadMap)
		if err != nil {
			return fmt.Errorf("failed to convert payload for snippet %s: %w", pointID, err)
		}

		points = append(points, &qdrant.PointStruct{
			Id:      &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointID}},
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: &qdrant.Vector{Data: s.Embedding}}},
			Payload: qdrantPayload,
		})
	}

	if len(points) == 0 {
		return nil // No valid points to upsert
	}

	_, err := c.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: c.collectionName,
		Points:         points,
		Wait:           proto.Bool(true), // Use proto.Bool for boolean pointers, ensure writes are acknowledged
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points to Qdrant: %w", err)
	}

	return nil
}

// Query searches for snippets similar to the given text embedding.
func (c *QdrantClient) Query(ctx context.Context, embedding domain.Embedding, k int) ([]domain.Snippet, error) {
	searchRequest := &qdrant.SearchPoints{
		CollectionName: c.collectionName,
		Vector:         embedding,
		Limit:          uint64(k),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	searchResult, err := c.client.Search(ctx, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to search points in Qdrant: %w", err)
	}

	snippets := make([]domain.Snippet, 0, len(searchResult.GetResult()))
	for _, hit := range searchResult.GetResult() {
		payload := hit.GetPayload()
		if payload == nil {
			continue
		}

		// Safely extract fields from the payload map
		content := payload["content"].GetStringValue()
		filePath := payload["file_path"].GetStringValue()
		startLine := payload["start_line"].GetIntegerValue()
		endLine := payload["end_line"].GetIntegerValue()

		symbols := []string{}
		if listVal, ok := payload["symbols"].GetKind().(*qdrant.Value_ListValue); ok && listVal != nil {
			for _, v := range listVal.ListValue.GetValues() {
				if s := v.GetStringValue(); s != "" {
					symbols = append(symbols, s)
				}
			}
		}

		pointID := ""
		if hit.GetId() != nil {
			if uuidVal, ok := hit.GetId().GetPointIdOptions().(*qdrant.PointId_Uuid); ok {
				pointID = uuidVal.Uuid
			}
		}

		// Extract custom metadata from payload
		metadata := make(map[string]string)
		for key, val := range payload {
			// Skip standard fields that we already extracted
			if key == "content" || key == "file_path" || key == "start_line" || key == "end_line" || key == "symbols" {
				continue
			}

			// Extract string values for metadata
			if strVal := val.GetStringValue(); strVal != "" {
				metadata[key] = strVal
			}
		}

		snippets = append(snippets, domain.Snippet{
			ID:        pointID,
			Content:   content,
			FilePath:  filePath,
			StartLine: int(startLine),
			EndLine:   int(endLine),
			Symbols:   symbols,
			Metadata:  metadata,
		})
	}

	return snippets, nil
}

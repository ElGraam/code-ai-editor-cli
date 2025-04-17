package infrastructure

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"

	"code-ai-editor/domain"
)

// AnthropicClient is a wrapper around the Anthropic API client.
// It provides a simplified interface for interacting with the Anthropic API.
type AnthropicClient struct {
	client *anthropic.Client
}

// NewAnthropicClient creates a new Anthropic client.
//
// It loads the environment variables from the .env.local file.
// It returns an error if the ANTHROPIC_API_KEY environment variable is not set.
//
// Returns:
//
//	*AnthropicClient: A pointer to the new Anthropic client.
//	error: An error if the client could not be created.
func NewAnthropicClient() (*AnthropicClient, error) {
	_ = godotenv.Load(".env.local")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic api key is not set")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicClient{
		client: &client,
	}, nil
}

// RunInference sends a conversation to the Anthropic API and returns the response.
//
// It takes a context, a slice of anthropic.MessageParam representing the conversation history,
// and a slice of domain.ToolDefinition representing the available tools.
// It converts the domain.ToolDefinition to anthropic.ToolParam and sends the request to the Anthropic API.
// It then prints the text content of the response to the console.
//
// Parameters:
//   - ctx: The context for the API call.
//   - conversation: A slice of anthropic.MessageParam representing the conversation history.
//   - tools: A slice of domain.ToolDefinition representing the available tools.
//
// Returns:
//   - *anthropic.Message: The response from the Anthropic API.
//   - error: An error if the API call fails.
func (a *AnthropicClient) RunInference(ctx context.Context, conversation []anthropic.MessageParam, tools []domain.ToolDefinition) (*anthropic.Message, error) {
	anthropicTools := []anthropic.ToolUnionParam{}
	for _, tool := range tools {
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: tool.InputSchema,
			},
		})
	}

	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_7SonnetLatest,
		MaxTokens: int64(1024),
		Messages:  conversation,
		Tools:     anthropicTools,
	})

	if err != nil {
		return nil, err
	}

	for _, content := range message.Content {
		if content.Type == "text" {
			fmt.Printf("\x1b[96mClaude\x1b[0m: %s\n", content.Text)
		}
	}

	return message, nil
}

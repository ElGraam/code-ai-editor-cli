package domain

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// UserMessageProvider is an interface that provides user messages.
// It is used to abstract the source of user messages, allowing the agent
// to receive input from various sources such as command line, GUI, or other
// input methods.
type UserMessageProvider interface {
	GetUserMessage() (string, bool)
}

// AIClient defines the interface for interacting with an AI model.
// It provides a method to run inference on a given conversation and set of tools.
type AIClient interface {
	RunInference(ctx context.Context, conversation []anthropic.MessageParam, tools []ToolDefinition) (*anthropic.Message, error)
}

// Agent orchestrates the interaction between the user, the AI client,
// and the available tools, potentially using a vector store for context.
type Agent struct {
	AIClient            AIClient
	UserMessageProvider UserMessageProvider
	ToolRepository      ToolRepository
	VectorStore         VectorStore     // Added for context retrieval
	EmbeddingClient     EmbeddingClient // Added for context retrieval
}

// NewAgent creates a new Agent with the provided dependencies.
func NewAgent(aiClient AIClient, userMessageProvider UserMessageProvider, toolRepository ToolRepository, vectorStore VectorStore, embeddingClient EmbeddingClient) *Agent {
	return &Agent{
		AIClient:            aiClient,
		UserMessageProvider: userMessageProvider,
		ToolRepository:      toolRepository,
		VectorStore:         vectorStore,
		EmbeddingClient:     embeddingClient,
	}
}

const maxContextLength = 1000 // Example: Limit context tokens/chars

// formatSnippets formats retrieved snippets into a string for the prompt context.
func formatSnippets(snippets []Snippet) string {
	if len(snippets) == 0 {
		return ""
	}
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Relevant code snippets based on your query:\n\n")
	currentLength := 0
	for _, s := range snippets {
		snippetHeader := fmt.Sprintf("--- File: %s (Lines: %d-%d) ---\n", s.FilePath, s.StartLine, s.EndLine)
		snippetContent := fmt.Sprintf("```go\n%s\n```\n\n", s.Content)

		if currentLength+len(snippetHeader)+len(snippetContent) > maxContextLength {
			contextBuilder.WriteString("... (omitting further snippets due to length limit)\n")
			break
		}

		contextBuilder.WriteString(snippetHeader)
		contextBuilder.WriteString(snippetContent)
		currentLength += len(snippetHeader) + len(snippetContent)
	}
	return contextBuilder.String()
}

// Run executes the agent's main loop, interacting with the user and the AI client.
//
// It implements the ReAct (Reasoning and Acting) pattern with these steps:
// 1. Observe: Get user input or tool results
// 2. Reason: Send information to AI for inference
// 3. Act: Execute tools based on AI's instructions
// 4. Repeat the cycle
//
// The loop continues until the user signals to stop providing input.
//
// Args:
//
//	ctx: The context for managing the execution of the agent.
//
// Returns:
//
//	An error if any step in the process fails, or nil if the agent completes
//	successfully.
func (a *Agent) Run(ctx context.Context) error {
	conversation := []anthropic.MessageParam{}

	for {
		// Step 1a: Observe - Get user input
		userInput, ok := a.UserMessageProvider.GetUserMessage()
		if !ok {
			break
		}

		// Step 1b: Context Retrieval
		var contextCode string
		if a.VectorStore != nil && a.EmbeddingClient != nil {
			// Generate embedding for user input
			log.Println("Generating embedding for user query...")
			embeddings, err := a.EmbeddingClient.GenerateEmbeddings(ctx, []string{userInput})
			if err != nil {
				log.Printf("Warning: Failed to generate embedding for query: %v\n", err)
				// Continue without context if embedding fails
			} else if len(embeddings) > 0 {
				// Query vector store
				log.Println("Querying vector store for relevant snippets...")
				const topK = 3 // Number of snippets to retrieve
				snippets, err := a.VectorStore.Query(ctx, embeddings[0], topK)
				if err != nil {
					log.Printf("Warning: Failed to query vector store: %v\n", err)
					// Continue without context if query fails
				} else {
					log.Printf("Retrieved %d snippets from vector store.\n", len(snippets))
					contextCode = formatSnippets(snippets)
				}
			}
		}

		// Add user message (and context if available) to conversation history
		messageContent := userInput
		if contextCode != "" {
			// Prepend context to the user's message or structure it differently
			messageContent = fmt.Sprintf("%s\n\nUser Query:\n%s", contextCode, userInput)
			fmt.Printf("\x1b[32mInjecting Context:\n%s\x1b[0m", contextCode) // Display injected context
		}
		userMessage := anthropic.NewUserMessage(anthropic.NewTextBlock(messageContent))
		conversation = append(conversation, userMessage)

		// Inner ReAct loop (Reason -> Act -> Observe Tool Results)
		for {
			// Step 2: Reason - Let the AI infer
			fmt.Print("\x1b[34mThinking...\x1b[0m\n")
			message, err := a.AIClient.RunInference(ctx, conversation, a.ToolRepository.GetAllTools())
			if err != nil {
				return err
			}
			conversation = append(conversation, message.ToParam())

			// Display AI's thought process (text response)
			for _, content := range message.Content {
				if content.Type == "text" {
					fmt.Printf("\x1b[36mClaude: %s\x1b[0m\n", content.Text)
				}
			}

			// Check if there are tool calls
			hasToolCalls := false
			toolResults := []anthropic.ContentBlockParamUnion{}

			for _, content := range message.Content {
				switch content.Type {
				case "tool_use":
					hasToolCalls = true
					// Step 3: Act - Execute the tool
					fmt.Printf("\x1b[33mExecuting: %s\x1b[0m\n", content.Name)
					result := a.ToolRepository.ExecuteTool(content.ID, content.Name, content.Input)
					toolResults = append(toolResults, result)
				}
			}

			// If there are no tool calls, exit internal ReAct loop (AI's thought is complete)
			if !hasToolCalls {
				break // Exit loop if only AI's text response
			}

			// Step 4: Observe - Observe the tool execution result
			fmt.Print("\x1b[32mObserving results...\x1b[0m\n")
			conversation = append(conversation, anthropic.NewUserMessage(toolResults...))
		}
	}

	return nil
}

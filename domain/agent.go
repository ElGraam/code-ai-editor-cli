package domain

import (
	"context"
	"fmt"

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
// and the available tools. It is responsible for receiving user messages,
// forwarding them to the AI client, and executing tools based on the AI's
// instructions. The Agent also manages the tool repository and provides
// user messages to the AI client.
type Agent struct {
	AIClient            AIClient
	UserMessageProvider UserMessageProvider
	ToolRepository      ToolRepository
}

// NewAgent creates a new Agent with the provided AI client, user message provider,
// and tool repository. It initializes the Agent struct with these dependencies,
// allowing the agent to interact with the AI, receive user messages, and access
// available tools.
//
// Parameters:
//   - aiClient: An AIClient instance for interacting with the AI model.
//   - userMessageProvider: A UserMessageProvider instance for retrieving user messages.
//   - toolRepository: A ToolRepository instance for accessing available tools.
//
// Returns:
//   - A pointer to a new Agent instance.
func NewAgent(aiClient AIClient, userMessageProvider UserMessageProvider, toolRepository ToolRepository) *Agent {
	return &Agent{
		AIClient:            aiClient,
		UserMessageProvider: userMessageProvider,
		ToolRepository:      toolRepository,
	}
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

	// ReAct loop internal cycle
	for {
		// Step 1: Observe - Get user input
		userInput, ok := a.UserMessageProvider.GetUserMessage()
		if !ok {
			break
		}

		// Add user message to conversation history
		userMessage := anthropic.NewUserMessage(anthropic.NewTextBlock(userInput))
		conversation = append(conversation, userMessage)

		// ReAct loop internal cycle
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

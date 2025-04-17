package application

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"code-ai-editor/domain"
)

// ChatbotService provides methods for interacting with the chatbot agent.
// It encapsulates the agent and provides a higher-level interface for sending
// messages and receiving responses.
type ChatbotService struct {
	agent *domain.Agent
}

// NewChatbotService creates a new ChatbotService with the given agent.
//
// Args:
//
//	agent: The agent to use for the chatbot service.
//
// Returns:
//
//	A new ChatbotService.
func NewChatbotService(agent *domain.Agent) *ChatbotService {
	return &ChatbotService{
		agent: agent,
	}
}

// CreateConsoleUserMessageProvider creates a new UserMessageProvider that reads messages from the console.
//
// It initializes a bufio.Scanner to read from standard input (os.Stdin) and returns a
// ConsoleUserMessageProvider instance configured with this scanner. This provider is
// responsible for retrieving user messages from the console.
func CreateConsoleUserMessageProvider() domain.UserMessageProvider {
	scanner := bufio.NewScanner(os.Stdin)

	return &ConsoleUserMessageProvider{
		scanner: scanner,
	}
}

// ConsoleUserMessageProvider provides user messages from the console.
// It uses a bufio.Scanner to read input from the standard input.
type ConsoleUserMessageProvider struct {
	scanner *bufio.Scanner
}

// GetUserMessage reads a message from the user via the console.
// It prints a prompt to the console, then waits for the user to enter a message.
// It returns the message entered by the user and a boolean indicating whether the read was successful.
// If the read was not successful (e.g., EOF is reached), it returns an empty string and false.
func (p *ConsoleUserMessageProvider) GetUserMessage() (string, bool) {
	fmt.Print("\x1b[95mYou\x1b[0m: ")
	if !p.scanner.Scan() {
		return "", false
	}
	return p.scanner.Text(), true
}

// StartChatbot starts the chatbot and runs the agent.
// It prints a message to the console indicating that the user can chat with Claude
// and use 'ctrl-c' to quit. It then calls the Run method of the agent to start the chatbot.
//
// Args:
//
//	ctx: The context for the chatbot.
//
// Returns:
//
//	An error if the chatbot fails to start.
func (s *ChatbotService) StartChatbot(ctx context.Context) error {
	fmt.Println("Chat with Claude (use 'ctrl-c' to quit)")
	return s.agent.Run(ctx)
}

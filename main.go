package main

import (
	"context"
	"fmt"
	"os"

	"code-ai-editor/application"
	"code-ai-editor/domain"
	"code-ai-editor/infrastructure"
)

// main is the entry point of the code-ai-editor-cli application.
// It initializes the Anthropic AI client, tool repository, user message provider,
// and the chatbot service. It then starts the chatbot and handles any errors that occur.
func main() {
	aiClient, err := infrastructure.NewAnthropicClient()
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
	}

	toolRepository := infrastructure.NewFileToolRepository()

	userMessageProvider := application.CreateConsoleUserMessageProvider()

	agent := domain.NewAgent(aiClient, userMessageProvider, toolRepository)

	chatbotService := application.NewChatbotService(agent)

	err = chatbotService.StartChatbot(context.Background())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
	}
}

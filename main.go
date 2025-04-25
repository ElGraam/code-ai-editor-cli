package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code-ai-editor/application"
	"code-ai-editor/domain"
	"code-ai-editor/infrastructure"
)

// main is the entry point of the code-ai-editor-cli application.
// It initializes the Anthropic AI client, tool repository, user message provider,
// and the chatbot service. It then starts the chatbot and handles any errors that occur.
// The application uses signal.NotifyContext to handle Ctrl+C gracefully.
func main() {
	// Create a done channel to ensure immediate exit on signal
	done := make(chan struct{})

	// Setup signal handling with both NotifyContext and explicit channel
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Additional signal handling to ensure immediate exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		// Ensure stdout is flushed
		os.Stdout.Sync()
		close(done)
		// Force exit after a short delay if normal exit doesn't work
		go func() {
			<-time.After(500 * time.Millisecond)
			os.Exit(0)
		}()
	}()

	aiClient, err := infrastructure.NewAnthropicClient()
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
	}

	toolRepository := infrastructure.NewFileToolRepository()

	userMessageProvider := application.CreateConsoleUserMessageProvider()

	agent := domain.NewAgent(aiClient, userMessageProvider, toolRepository)

	chatbotService := application.NewChatbotService(agent)

	// Run chatbot in a goroutine so we can monitor for done signal
	errChan := make(chan error, 1)
	go func() {
		errChan <- chatbotService.StartChatbot(ctx)
	}()

	// Wait for either completion or interruption
	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Printf("Error: %s\n", err.Error())
			os.Exit(1)
		}
	case <-done:
		// Immediate exit path from signal handler
	}

	// Exit normally
	fmt.Println("\nGoodbye!")
}

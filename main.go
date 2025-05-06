package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code-ai-editor/application"
	"code-ai-editor/domain"
	"code-ai-editor/infrastructure"
	infra_embedding "code-ai-editor/infrastructure/embedding"
	infra_vectorstore "code-ai-editor/infrastructure/vectorstore"

	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

// Command-line flags
var (
	indexFlag = flag.Bool("index", false, "Index files in the workspace directory for vector search")
)

// main is the entry point of the code-ai-editor-cli application.
// It initializes the Anthropic AI client, tool repository, user message provider,
// and the chatbot service. It then starts the chatbot and handles any errors that occur.
// The application uses signal.NotifyContext to handle Ctrl+C gracefully.
func main() {
	flag.Parse()

	// Load environment variables from .env.local file
	if err := godotenv.Load(".env.local"); err != nil {
		log.Println("Warning: Could not load .env.local file. Using environment variables directly.")
	}

	// Create a done channel to ensure immediate exit on signal
	done := make(chan struct{})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		os.Stdout.Sync()
		close(done) // Corrected: Keep done channel
		go func() {
			<-time.After(500 * time.Millisecond)
			os.Exit(0)
		}()
	}()

	// Initialize Vector Store (Qdrant)
	vectorStore, err := infra_vectorstore.NewQdrantClient()
	if err != nil {
		log.Fatalf("Error initializing Qdrant client: %s\n", err.Error())
	}

	// Initialize Embedding Client (OpenAI)
	embeddingModel := openai.SmallEmbedding3
	embeddingClient, err := infra_embedding.NewOpenAIEmbeddingClient(embeddingModel)
	if err != nil {
		if os.Getenv("OPENAI_API_KEY") == "" {
			log.Println("Warning: OPENAI_API_KEY not set. Context retrieval via embeddings will be disabled.")
			embeddingClient = nil
		} else {
			log.Fatalf("Error initializing OpenAI client: %s\n", err.Error())
		}
	}

	// Initialize Code Parser
	codeParser := domain.NewGoCodeParser()

	// Handle indexing if --index flag is provided
	if *indexFlag {
		if embeddingClient == nil {
			log.Fatalf("Cannot perform indexing without a valid embedding client (OPENAI_API_KEY missing?).")
		}

		// Always index the workspace directory
		workspaceDir := "./workspace"

		// Ensure the workspace directory exists
		if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
			log.Printf("Workspace directory does not exist, creating: %s\n", workspaceDir)
			if err := os.MkdirAll(workspaceDir, 0755); err != nil {
				log.Fatalf("Failed to create workspace directory: %s\n", err.Error())
			}
		}

		indexingService := application.NewIndexingService(codeParser, embeddingClient, vectorStore)
		log.Printf("Starting indexing for workspace directory: %s\n", workspaceDir)
		if err := indexingService.IndexDirectory(ctx, workspaceDir); err != nil {
			log.Fatalf("Error during indexing: %s\n", err.Error())
		}
		log.Println("Indexing complete.")
		return // Exit after indexing
	}

	// --- Initialize core chatbot components ---
	aiClient, err := infrastructure.NewAnthropicClient()
	if err != nil {
		log.Fatalf("Error initializing Anthropic client: %s\n", err.Error())
	}

	toolRepository := infrastructure.NewFileToolRepository(vectorStore, embeddingClient)

	userMessageProvider := application.CreateConsoleUserMessageProvider()

	// Pass VectorStore and EmbeddingClient to the Agent
	// Corrected: Pass all required arguments
	agent := domain.NewAgent(aiClient, userMessageProvider, toolRepository, vectorStore, embeddingClient)

	chatbotService := application.NewChatbotService(agent)

	errChan := make(chan error, 1)
	go func() {
		errChan <- chatbotService.StartChatbot(ctx)
	}()

	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Printf("Error: %s\n", err.Error())
			os.Exit(1)
		}
	case <-done: // Corrected: Keep done channel usage
		// Immediate exit path from signal handler
	}

	fmt.Println("\nGoodbye!")
}

package application

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"code-ai-editor/domain"

	"github.com/google/uuid"
)

// IndexingService handles the process of parsing, embedding, and indexing code files.
type IndexingService struct {
	parser      domain.CodeParser
	embedder    domain.EmbeddingClient
	vectorStore domain.VectorStore
}

// NewIndexingService creates a new IndexingService.
func NewIndexingService(parser domain.CodeParser, embedder domain.EmbeddingClient, vectorStore domain.VectorStore) *IndexingService {
	return &IndexingService{
		parser:      parser,
		embedder:    embedder,
		vectorStore: vectorStore,
	}
}

// IndexDirectory walks through the specified directory, finds all files, parses them,
// generates embeddings, and upserts the snippets into the vector store.
func (s *IndexingService) IndexDirectory(ctx context.Context, rootDir string) error {
	log.Printf("Starting indexing for directory: %s\n", rootDir)
	var allSnippets []domain.Snippet

	// Track file count statistics
	fileStats := make(map[string]int)
	var totalFileCount int

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error accessing path %q: %v\n", path, err)
			return err
		}
		if ctx.Err() != nil {
			log.Println("Context cancelled, stopping walk.")
			return ctx.Err() // Stop walking if context is cancelled
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Track file extension statistics
		ext := strings.ToLower(filepath.Ext(path))
		fileStats[ext]++
		totalFileCount++

		// Process the file based on extension
		var snippets []domain.Snippet
		var parseErr error

		switch {
		case strings.HasSuffix(path, ".go"):
			// Parse Go files using the specialized Go parser
			log.Printf("Parsing Go file: %s\n", path)
			snippets, parseErr = s.parser.Parse(ctx, path)
		default:
			// For other file types, create a simple snippet with the entire file content
			log.Printf("Processing file: %s\n", path)
			snippet, err := s.createFileSnippet(path)
			if err == nil {
				snippets = []domain.Snippet{snippet}
			} else {
				parseErr = err
			}
		}

		if parseErr != nil {
			log.Printf("Error processing file %s: %v\n", path, parseErr)
			return nil // Continue walking
		}

		allSnippets = append(allSnippets, snippets...)
		return nil
	})

	if err != nil && err != context.Canceled {
		return fmt.Errorf("error walking directory %s: %w", rootDir, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Log file type statistics
	log.Printf("Processed %d total files:\n", totalFileCount)
	for ext, count := range fileStats {
		if ext == "" {
			ext = "(no extension)"
		}
		log.Printf("  %s: %d files\n", ext, count)
	}

	if len(allSnippets) == 0 {
		log.Println("No files parsed into snippets.")
		return nil
	}

	log.Printf("Created %d snippets. Generating embeddings...\n", len(allSnippets))

	// Extract content for embedding
	textsToEmbed := make([]string, len(allSnippets))
	for i, snippet := range allSnippets {
		// Combine relevant info for better embedding
		if len(snippet.Symbols) > 0 {
			textsToEmbed[i] = fmt.Sprintf("File: %s\nSymbols: %s\n%s",
				snippet.FilePath, strings.Join(snippet.Symbols, ", "), snippet.Content)
		} else {
			textsToEmbed[i] = fmt.Sprintf("File: %s\n%s", snippet.FilePath, snippet.Content)
		}
	}

	// Process embeddings in batches to prevent memory issues
	batchSize := 100
	for i := 0; i < len(allSnippets); i += batchSize {
		end := i + batchSize
		if end > len(allSnippets) {
			end = len(allSnippets)
		}

		log.Printf("Generating embeddings for batch %d/%d (snippets %d-%d)...\n",
			(i/batchSize)+1, (len(allSnippets)+batchSize-1)/batchSize, i+1, end)

		batchTexts := textsToEmbed[i:end]
		batchEmbeddings, err := s.embedder.GenerateEmbeddings(ctx, batchTexts)
		if err != nil {
			return fmt.Errorf("error generating embeddings for batch %d-%d: %w", i+1, end, err)
		}

		if len(batchEmbeddings) != len(batchTexts) {
			return fmt.Errorf("mismatch between number of batch texts (%d) and embeddings (%d)",
				len(batchTexts), len(batchEmbeddings))
		}

		// Assign embeddings back to snippets
		for j := range batchEmbeddings {
			allSnippets[i+j].Embedding = batchEmbeddings[j]
		}

		// Upsert batch immediately to reduce memory usage
		log.Printf("Upserting batch of %d snippets to vector store...\n", len(batchEmbeddings))
		err = s.vectorStore.Upsert(ctx, allSnippets[i:end])
		if err != nil {
			return fmt.Errorf("error upserting batch %d-%d: %w", i+1, end, err)
		}
	}

	log.Printf("Successfully indexed %d snippets from %s\n", len(allSnippets), rootDir)
	return nil
}

// createFileSnippet creates a snippet from a non-Go file by reading its entire content.
func (s *IndexingService) createFileSnippet(filePath string) (domain.Snippet, error) {
	content, err := s.readFileContent(filePath)
	if err != nil {
		return domain.Snippet{}, err
	}

	// Generate a proper UUID instead of a filename-based ID
	id := uuid.New().String()

	// For text files, limit content size to prevent issues with large files
	maxContentSize := 10000 // Maximum number of characters
	if len(content) > maxContentSize {
		content = content[:maxContentSize] + "... [content truncated]"
	}

	return domain.Snippet{
		ID:        id,
		Content:   content,
		FilePath:  filePath,
		StartLine: 1,
		EndLine:   len(strings.Split(content, "\n")),
		// No symbols for non-code files
		Symbols: []string{},
		// Embedding will be added later
		Metadata: map[string]string{
			"file_type": strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), "."),
			"file_name": filepath.Base(filePath), // Store the filename in metadata instead
		},
	}, nil
}

// readFileContent reads the content of a file and returns it as a string.
func (s *IndexingService) readFileContent(filePath string) (string, error) {
	// Read the file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Skip binary files or very large files
	if len(data) > 10*1024*1024 { // 10MB limit
		return "", fmt.Errorf("file too large (>10MB): %s", filePath)
	}

	// Get file extension
	ext := strings.ToLower(filepath.Ext(filePath))

	// Known text file extensions that should never be treated as binary
	knownTextExtensions := []string{".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".py", ".go",
		".c", ".cpp", ".h", ".java", ".sh", ".bat", ".ps1", ".yaml", ".yml", ".toml", ".ini", ".cfg",
		".config", ".properties", ".env", ".example", ".log", ".gitignore", ".csv", ".tsv"}

	// Check if it's a known text extension
	isKnownExt := false
	for _, textExt := range knownTextExtensions {
		if ext == textExt {
			isKnownExt = true
			break
		}
	}

	// Skip binary detection for known text extensions
	if !isKnownExt && isBinary(data) {
		return "", fmt.Errorf("skipping binary file: %s", filePath)
	}

	return string(data), nil
}

// isBinary does a check to determine if data might be a binary file.
func isBinary(data []byte) bool {
	// Check first N bytes for null bytes or other binary indicators
	checkSize := 1000
	if len(data) < checkSize {
		checkSize = len(data)
	}

	// UTF-8 BOM detection (EF BB BF)
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		// It's a UTF-8 text file with BOM, not binary
		return false
	}

	// For short files (less than 32 bytes), assume they're text
	if checkSize < 32 {
		return false
	}

	// Count control characters and extended ASCII
	controlCount := 0
	extendedASCIICount := 0
	nullCount := 0

	for i := 0; i < checkSize; i++ {
		b := data[i]
		if b == 0 {
			nullCount++
		} else if b < 9 || (b > 13 && b < 32 && b != 27) {
			// Control characters except tab, LF, CR, etc.
			controlCount++
		} else if b >= 128 && b <= 159 {
			// Extended ASCII control codes
			extendedASCIICount++
		}
	}

	// If the file content suggests it's text, be more lenient
	if isKnownTextFile(data) {
		// For known text files, only detect as binary if there are many null bytes
		return nullCount > checkSize/50
	}

	// More aggressive binary detection for unknown file types
	// Heuristic: If more than 0.1% null bytes or more than 1% control characters, consider it binary
	return (nullCount > checkSize/1000) || (controlCount > checkSize/100) || (extendedASCIICount > checkSize/50)
}

// isKnownTextFile checks if the file has a known text file signature
func isKnownTextFile(data []byte) bool {
	// Check for common text file markers
	if len(data) > 0 {
		// Check for various text markers that indicate this is likely text
		textMarkers := []string{
			"<!DOCTYPE", "<html", "<?xml", "{", "[", "//", "/*", "#!", "import ", "package ", "using ",
			"function ", "class ", "def ", "var ", "const ", "let ", "from ", "# ", "// ", "/* ", "; ", "' ",
		}

		dataStart := string(data[:min(100, len(data))])
		dataStartLower := strings.ToLower(dataStart)

		for _, marker := range textMarkers {
			if strings.Contains(dataStartLower, strings.ToLower(marker)) {
				return true
			}
		}
	}

	return false
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

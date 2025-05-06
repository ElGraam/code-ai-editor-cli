package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/invopop/jsonschema"

	"code-ai-editor/domain"
)

// Helper function to validate and resolve paths within the workspace
func resolveWorkspacePath(relativePath string) (string, error) {
	// Get current working directory (project root)
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	workspaceDir := filepath.Join(cwd, "workspace")

	// Ensure the workspace directory exists, create if not
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create workspace directory '%s': %w", workspaceDir, err)
		}
	}

	// Clean the user-provided path and join it with the workspace directory
	// Users should provide paths relative to workspace, e.g., "my_subdir/my_file.go"
	// No need to prefix with "workspace/" in the input.
	cleanedRelativePath := filepath.Clean(relativePath)

	// Prevent path traversal attempts like "../sensitive_file"
	if strings.HasPrefix(cleanedRelativePath, "..") {
		return "", fmt.Errorf("invalid path: '%s' attempts to traverse outside the workspace", relativePath)
	}

	fullPath := filepath.Join(workspaceDir, cleanedRelativePath)

	// Get absolute paths for robust comparison
	absWorkspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for workspace: %w", err)
	}
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for target: %w", err)
	}

	// Final check: Ensure the resolved absolute path is truly within the workspace directory
	if !strings.HasPrefix(absFullPath, absWorkspaceDir) {
		return "", fmt.Errorf("invalid path: '%s' resolves outside the workspace directory", relativePath)
	}

	return absFullPath, nil
}

// FileToolRepository manages tool definitions and provides interfaces to interact with the BraveClient API.
// It holds a collection of tool definitions in the 'tools' slice and an instance of BraveClient in the 'braveClient'
// field, which is used to communicate with external services.
type FileToolRepository struct {
	tools           []domain.ToolDefinition
	braveClient     *BraveClient
	vectorStore     domain.VectorStore
	embeddingClient domain.EmbeddingClient
}

// NewFileToolRepository creates and returns a new FileToolRepository.
// It initializes the necessary file tool definitions used for reading,
// listing, and editing files. Additionally, if the creation of a Brave
// web search client is successful, it also adds a web search tool to the
// repository. The returned repository contains both the initialized tools
// and the Brave client (if available).
func NewFileToolRepository(vectorStore domain.VectorStore, embeddingClient domain.EmbeddingClient) *FileToolRepository {
	braveClient, err := NewBraveClient()
	var searchTool domain.ToolDefinition
	if err == nil {
		searchTool = SearchWebDefinition(braveClient)
	}

	tools := []domain.ToolDefinition{
		ReadFileDefinition(),
		ListFilesDefinition(),
		EditFileDefinition(),
		CreateFileDefinition(),
	}

	if err == nil {
		tools = append(tools, searchTool)
	}

	// Add Qdrant tools if vector store and embedding client are available
	if vectorStore != nil && embeddingClient != nil {
		tools = append(tools,
			QdrantSearchDefinition(vectorStore, embeddingClient),
			QdrantUpsertDefinition(vectorStore, embeddingClient),
		)
	}

	return &FileToolRepository{
		tools:           tools,
		braveClient:     braveClient,
		vectorStore:     vectorStore,
		embeddingClient: embeddingClient,
	}
}

// GetAllTools returns a slice of ToolDefinition representing all the tools
// currently managed by the repository. It retrieves the preloaded list of tools,
// which may be empty if no tools have been initialized.
func (r *FileToolRepository) GetAllTools() []domain.ToolDefinition {
	return r.tools
}

// FindToolByName searches for a tool by its name in the repository.
// It returns the ToolDefinition if found, along with a boolean indicating success.
// If the tool is not found, it returns an empty ToolDefinition and false.
func (r *FileToolRepository) FindToolByName(name string) (domain.ToolDefinition, bool) {
	for _, tool := range r.tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return domain.ToolDefinition{}, false
}

// ExecuteTool executes a tool based on the provided name and input.
// It searches for the tool definition by name and, if found, calls the tool's function with the input.
// If the tool is not found, it returns a result indicating the tool was not found.
// If an error occurs during execution, it returns the error message.
// Parameters:
//   - id: The identifier for the tool execution.
//   - name: The name of the tool to execute.
//   - input: The JSON-encoded input for the tool.
//
// Returns:
//
//	anthropic.ContentBlockParamUnion: The result of the tool execution, which may include an error message.
func (r *FileToolRepository) ExecuteTool(id, name string, input json.RawMessage) anthropic.ContentBlockParamUnion {
	toolDef, found := r.FindToolByName(name)
	if !found {
		return anthropic.NewToolResultBlock(id, "tool not found", true)
	}

	fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", name, input)
	response, err := toolDef.Function(input)
	if err != nil {
		return anthropic.NewToolResultBlock(id, fmt.Sprintf("Error executing tool '%s': %v", name, err), true)
	}
	return anthropic.NewToolResultBlock(id, response, false)
}

// GenerateSchema creates a JSON schema for the specified type T.
// It uses the jsonschema.Reflector to reflect the properties of the type.
// The generated schema does not allow additional properties and does not create references.
// Returns:
//
//	anthropic.ToolInputSchemaParam: The generated JSON schema for the input type.
func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T

	schema := reflector.Reflect(v)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
		Type:       "object",
	}
}

// SearchWebInput defines the input parameters for the search_web tool.
// It contains a single field, Query, which represents the search query to execute.
type SearchWebInput struct {
	Query string `json:"query" jsonschema:"required,description=The search query to execute."`
}

// SearchWebDefinition returns a ToolDefinition for the "search_web" tool, which allows searching the web using the Brave Search API.
// This tool should be used when you need to find information on the internet.
// It includes the input schema for the search query and a function that executes the search.
func SearchWebDefinition(braveClient *BraveClient) domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "search_web",
		Description: "Search the web using Brave Search API. Use this when you need to find information on the internet.",
		InputSchema: GenerateSchema[SearchWebInput](),
		Function: func(input json.RawMessage) (string, error) {
			return SearchWeb(braveClient, input)
		},
	}
}

// SearchWeb executes a web search using the Brave Search API.
// It takes a BraveClient instance and a JSON-encoded input containing the search query.
// If the input is invalid or the query is empty, it returns an error.
// On success, it returns the search results as a formatted JSON string.
//
// Parameters:
//   - braveClient: An instance of BraveClient used to perform the search.
//   - input: A JSON-encoded raw message containing the search query.
//
// Returns:
//   - A string containing the formatted JSON results of the search.
//   - An error if the search fails or if the input is invalid.
func SearchWeb(braveClient *BraveClient, input json.RawMessage) (string, error) {
	if braveClient == nil {
		return "", fmt.Errorf("Brave Search API client is not configured (BRAVE_API_KEY missing?)")
	}
	var searchWebInput SearchWebInput
	err := json.Unmarshal(input, &searchWebInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for search_web: %w", err)
	}

	if searchWebInput.Query == "" {
		return "", fmt.Errorf("query is required for search_web")
	}

	results, err := braveClient.Search(searchWebInput.Query)
	if err != nil {
		return "", fmt.Errorf("Brave Search API error: %w", err)
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal search results: %w", err)
	}

	return string(resultJSON), nil
}

// ReadFileInput represents the input required to read a file from the workspace directory.
// It contains the path relative to the workspace root.
type ReadFileInput struct {
	Path string `json:"path" jsonschema:"required,description=The path of the file relative to the workspace directory."`
}

// ReadFileDefinition returns a ToolDefinition for the "read_file" tool, which allows reading the contents
// of a specified file within the workspace directory. This tool should be used to inspect the contents of files.
// The path must be relative to the workspace directory.
func ReadFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file within the workspace directory. Provide the path relative to the workspace root (e.g., 'subdir/my_file.txt'). Do not use directory names.",
		InputSchema: GenerateSchema[ReadFileInput](),
		Function:    ReadFile,
	}
}

// ReadFile reads the contents of a file specified in the input JSON, ensuring it's within the workspace.
// The input must contain the file path relative to the workspace.
// It returns the file contents as a string, or an error if the path is invalid or the file cannot be read.
func ReadFile(input json.RawMessage) (string, error) {
	var readFileInput ReadFileInput
	err := json.Unmarshal(input, &readFileInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for read_file: %w", err)
	}
	if readFileInput.Path == "" {
		return "", fmt.Errorf("path is required for read_file")
	}

	absPath, err := resolveWorkspacePath(readFileInput.Path)
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found at path '%s' within workspace", readFileInput.Path)
		}
		return "", fmt.Errorf("failed to stat file '%s': %w", readFileInput.Path, err)
	}
	if fileInfo.IsDir() {
		return "", fmt.Errorf("path '%s' is a directory, not a file", readFileInput.Path)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", readFileInput.Path, err)
	}
	return string(content), nil
}

// ListFilesInput represents the input parameters for listing files in a directory within the workspace.
// The Path field specifies an optional path relative to the workspace root.
// If Path is empty or ".", the workspace root directory is listed.
type ListFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional path relative to the workspace root. Defaults to the workspace root if empty or '.'."`
}

// ListFilesDefinition returns a ToolDefinition for listing files and directories within the workspace.
// It lists files in the specified path relative to the workspace root.
func ListFilesDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "list_files",
		Description: "List files and directories within the workspace directory. Provide the path relative to the workspace root (e.g., 'subdir' or '.'). Defaults to the workspace root if no path is provided.",
		InputSchema: GenerateSchema[ListFilesInput](),
		Function:    ListFiles,
	}
}

// ListFiles lists files and directories within a specified path inside the workspace.
// The input path is relative to the workspace root. Defaults to the workspace root if empty.
// Returns a JSON-encoded list of relative paths (directories suffixed with '/').
func ListFiles(input json.RawMessage) (string, error) {
	var listFilesInput ListFilesInput
	if len(input) > 0 && string(input) != "null" && string(input) != "{}" {
		err := json.Unmarshal(input, &listFilesInput)
		if err != nil {
			return "", fmt.Errorf("invalid input format for list_files: %w", err)
		}
	}
	relativePath := listFilesInput.Path
	if relativePath == "" {
		relativePath = "."
	}

	absPath, err := resolveWorkspacePath(relativePath)
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("directory not found at path '%s' within workspace", relativePath)
		}
		return "", fmt.Errorf("failed to stat directory '%s': %w", relativePath, err)
	}
	if !fileInfo.IsDir() {
		return "", fmt.Errorf("path '%s' is not a directory", relativePath)
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory '%s': %w", relativePath, err)
	}

	var results []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		results = append(results, filepath.ToSlash(filepath.Join(relativePath, name)))
	}

	resultJSON, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("failed to marshal file list: %w", err)
	}

	return string(resultJSON), nil
}

// EditFileInput defines the input parameters for the edit_file tool.
// It requires the file path (relative to workspace), the exact old string to find,
// and the new string to replace it with. The old string must have exactly one match.
type EditFileInput struct {
	Path   string `json:"path" jsonschema:"required,description=The path to the file relative to the workspace directory."`
	OldStr string `json:"old_str" jsonschema:"required,description=Text to search for - must match exactly and must only have one match exactly."`
	NewStr string `json:"new_str" jsonschema:"required,description=Text to replace old_str with."`
}

// EditFileDefinition returns the tool definition for editing a file within the workspace.
func EditFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "edit_file",
		Description: "Search for an exact string ('old_str') in a file within the workspace (specified by 'path' relative to workspace root) and replace its single occurrence with 'new_str'. Fails if 'old_str' is not found or found multiple times.",
		InputSchema: GenerateSchema[EditFileInput](),
		Function:    EditFile,
	}
}

// EditFile reads a file, replaces exactly one occurrence of oldStr with newStr, and writes it back.
// Paths are resolved relative to the workspace directory.
func EditFile(input json.RawMessage) (string, error) {
	var editFileInput EditFileInput
	err := json.Unmarshal(input, &editFileInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for edit_file: %w", err)
	}

	if editFileInput.Path == "" || editFileInput.OldStr == "" {
		return "", fmt.Errorf("path and old_str are required for edit_file")
	}

	absPath, err := resolveWorkspacePath(editFileInput.Path)
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found at path '%s' within workspace", editFileInput.Path)
		}
		return "", fmt.Errorf("failed to stat file '%s': %w", editFileInput.Path, err)
	}
	if fileInfo.IsDir() {
		return "", fmt.Errorf("path '%s' is a directory, cannot edit", editFileInput.Path)
	}

	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", editFileInput.Path, err)
	}
	content := string(contentBytes)

	count := strings.Count(content, editFileInput.OldStr)
	if count == 0 {
		return "", fmt.Errorf("string '%s' not found in file '%s'", editFileInput.OldStr, editFileInput.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("string '%s' found multiple times (%d) in file '%s', expected exactly one", editFileInput.OldStr, count, editFileInput.Path)
	}

	newContent := strings.Replace(content, editFileInput.OldStr, editFileInput.NewStr, 1)

	err = os.WriteFile(absPath, []byte(newContent), fileInfo.Mode())
	if err != nil {
		return "", fmt.Errorf("failed to write changes to file '%s': %w", editFileInput.Path, err)
	}

	return fmt.Sprintf("Successfully edited file '%s'", editFileInput.Path), nil
}

// CreateFileInput defines the input for creating a new file within the workspace.
type CreateFileInput struct {
	Path    string `json:"path" jsonschema:"required,description=The path relative to the workspace where the file should be created (including filename)."`
	Content string `json:"content" jsonschema:"required,description=The content to write to the new file."`
}

// CreateFileDefinition returns the tool definition for creating a new file within the workspace.
func CreateFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "create_file",
		Description: "Create a new file with the specified content at a path relative to the workspace root. Fails if the file already exists or the path is invalid.",
		InputSchema: GenerateSchema[CreateFileInput](),
		Function:    CreateFile,
	}
}

// CreateFile creates a new file at the specified path within the workspace.
// Fails if the file already exists or the path is invalid.
func CreateFile(input json.RawMessage) (string, error) {
	var createFileInput CreateFileInput
	err := json.Unmarshal(input, &createFileInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for create_file: %w", err)
	}

	if createFileInput.Path == "" {
		return "", fmt.Errorf("path is required for create_file")
	}

	absPath, err := resolveWorkspacePath(createFileInput.Path)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(absPath); err == nil {
		return "", fmt.Errorf("file already exists at path '%s'", createFileInput.Path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to check file status for '%s': %w", createFileInput.Path, err)
	}

	parentDir := filepath.Dir(absPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directory for '%s': %w", createFileInput.Path, err)
	}

	err = os.WriteFile(absPath, []byte(createFileInput.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create or write file '%s': %w", createFileInput.Path, err)
	}

	return fmt.Sprintf("Successfully created file '%s'", createFileInput.Path), nil
}

// QdrantSearchInput defines the input for searching the Qdrant vector store.
type QdrantSearchInput struct {
	Query string `json:"query" jsonschema:"required,description=The search query text to be embedded for searching."`
	K     int    `json:"k" jsonschema:"required,description=The number of nearest neighbors to return."`
}

// QdrantSearchDefinition returns a tool definition for searching in the Qdrant vector store.
func QdrantSearchDefinition(vectorStore domain.VectorStore, embeddingClient domain.EmbeddingClient) domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "qdrant_search",
		Description: "Searches for relevant information in the Qdrant vector store (long-term memory or RAG context) using a query string.",
		InputSchema: GenerateSchema[QdrantSearchInput](),
		Function: func(input json.RawMessage) (string, error) {
			return QdrantSearch(vectorStore, embeddingClient, input)
		},
	}
}

// QdrantSearch performs a search in the Qdrant vector store.
// If vector search fails, it falls back to searching fallback files.
func QdrantSearch(vectorStore domain.VectorStore, embeddingClient domain.EmbeddingClient, input json.RawMessage) (string, error) {
	if vectorStore == nil || embeddingClient == nil {
		return "", fmt.Errorf("vector store or embedding client is not configured")
	}

	var searchInput QdrantSearchInput
	err := json.Unmarshal(input, &searchInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for qdrant_search: %w", err)
	}

	if searchInput.Query == "" {
		return "", fmt.Errorf("query is required for qdrant_search")
	}

	if searchInput.K <= 0 {
		searchInput.K = 5 // Default to 5 if not specified or invalid
	}

	// Create embedding for the query
	fmt.Println("Generating embeddings for search query...")
	embeddings, err := embeddingClient.GenerateEmbeddings(context.Background(), []string{searchInput.Query})
	if err != nil {
		fmt.Printf("Error generating embeddings: %v\n", err)
		return fallbackToFileSearch(searchInput.Query)
	}

	if len(embeddings) == 0 {
		fmt.Println("No embeddings generated for search query")
		return fallbackToFileSearch(searchInput.Query)
	}

	// Search in vector store
	fmt.Println("Attempting to search in vector store...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := vectorStore.Query(ctx, embeddings[0], searchInput.K)
	if err != nil {
		fmt.Printf("Error searching in vector store: %v\n", err)
		return fallbackToFileSearch(searchInput.Query)
	}

	if len(results) == 0 {
		fmt.Println("No results found in vector store, falling back to file search")
		return fallbackToFileSearch(searchInput.Query)
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal search results: %w", err)
	}

	return string(resultJSON), nil
}

// fallbackToFileSearch searches for relevant information in fallback files
func fallbackToFileSearch(query string) (string, error) {
	fmt.Println("Falling back to file search...")

	// Get workspace directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	workspaceDir := filepath.Join(cwd, "workspace")

	// Find all fallback files
	pattern := filepath.Join(workspaceDir, "vector_store_fallback_*.txt")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to list fallback files: %w", err)
	}

	if len(files) == 0 {
		return "No fallback files found. No search results available.", nil
	}

	// Read all files and perform a basic keyword search
	type SearchResult struct {
		Filename    string  `json:"filename"`
		Content     string  `json:"content"`
		Relevance   float64 `json:"relevance"`
		MatchedLine string  `json:"matched_line,omitempty"`
	}

	var results []SearchResult

	queryTerms := strings.Fields(strings.ToLower(query))
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", file, err)
			continue
		}

		contentStr := string(content)

		// Calculate a simple relevance score based on term frequency
		var score float64
		var bestLine string
		var bestLineScore float64

		lines := strings.Split(contentStr, "\n")
		for _, line := range lines {
			lineLower := strings.ToLower(line)
			lineScore := 0.0

			for _, term := range queryTerms {
				count := strings.Count(lineLower, term)
				if count > 0 {
					lineScore += float64(count)
				}
			}

			score += lineScore

			// Keep track of the most relevant line
			if lineScore > bestLineScore {
				bestLineScore = lineScore
				bestLine = line
			}
		}

		// If there is any relevance, add to results
		if score > 0 {
			filename := filepath.Base(file)
			result := SearchResult{
				Filename:    filename,
				Content:     contentStr,
				Relevance:   score,
				MatchedLine: bestLine,
			}
			results = append(results, result)
		}
	}

	// Sort results by relevance (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	// Limit results for better readability
	if len(results) > 5 {
		results = results[:5]
	}

	if len(results) == 0 {
		return "No relevant information found in fallback files.", nil
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal file search results: %w", err)
	}

	return string(resultJSON), nil
}

// QdrantUpsertInput defines the input for upserting into the Qdrant vector store.
type QdrantUpsertInput struct {
	TextContent string            `json:"text_content" jsonschema:"required,description=The text content to be embedded and stored."`
	Metadata    map[string]string `json:"metadata,omitempty" jsonschema:"description=A map of key-value pairs for metadata associated with the content."`
}

// QdrantUpsertDefinition returns a tool definition for upserting into the Qdrant vector store.
func QdrantUpsertDefinition(vectorStore domain.VectorStore, embeddingClient domain.EmbeddingClient) domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "qdrant_upsert",
		Description: "Upserts (inserts or updates) information into the Qdrant vector store (long-term memory or RAG context).",
		InputSchema: GenerateSchema[QdrantUpsertInput](),
		Function: func(input json.RawMessage) (string, error) {
			return QdrantUpsert(vectorStore, embeddingClient, input)
		},
	}
}

// QdrantUpsert performs an upsert operation in the Qdrant vector store.
// If the upsert to vector store fails, it automatically falls back to saving the content as a file.
func QdrantUpsert(vectorStore domain.VectorStore, embeddingClient domain.EmbeddingClient, input json.RawMessage) (string, error) {
	if vectorStore == nil {
		fmt.Println("Error: Vector store is nil")
		return "", fmt.Errorf("vector store is not configured")
	}

	if embeddingClient == nil {
		fmt.Println("Error: Embedding client is nil")
		return "", fmt.Errorf("embedding client is not configured")
	}

	var upsertInput QdrantUpsertInput
	err := json.Unmarshal(input, &upsertInput)
	if err != nil {
		return "", fmt.Errorf("invalid input format for qdrant_upsert: %w", err)
	}

	if upsertInput.TextContent == "" {
		return "", fmt.Errorf("text_content is required for qdrant_upsert")
	}

	// Create embedding for the text content
	fmt.Println("Generating embeddings via OpenAI API...")

	// Using timeout context for embedding generation
	embedCtx, embedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer embedCancel()

	embeddings, err := embeddingClient.GenerateEmbeddings(embedCtx, []string{upsertInput.TextContent})
	if err != nil {
		fmt.Printf("Error generating embeddings: %v\n", err)
		return fallbackToFileStore(upsertInput)
	}

	if len(embeddings) == 0 {
		fmt.Println("No embeddings generated - empty result from embedding client")
		return fallbackToFileStore(upsertInput)
	}

	if len(embeddings[0]) == 0 {
		fmt.Println("Generated embedding has zero dimensions - invalid embedding")
		return fallbackToFileStore(upsertInput)
	}

	fmt.Printf("Successfully generated embedding with %d dimensions\n", len(embeddings[0]))

	// Create a UUID for the vector
	id := uuid.New().String()
	fmt.Printf("Created UUID: %s\n", id)

	// Prepare the point with embedding, payload and ID
	point := domain.Snippet{
		ID:        id,
		Content:   upsertInput.TextContent,
		Embedding: embeddings[0],
		Metadata:  upsertInput.Metadata,
		// Initialize other required fields with empty/zero values
		FilePath:  "",
		StartLine: 0,
		EndLine:   0,
		Symbols:   []string{},
	}

	// Log the prepared point
	fmt.Printf("Prepared snippet with ID: %s, Embedding length: %d, Content length: %d bytes\n",
		point.ID, len(point.Embedding), len(point.Content))

	if len(point.Metadata) > 0 {
		fmt.Println("Metadata fields:")
		for k, v := range point.Metadata {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}

	// Upsert the point
	fmt.Println("Attempting to upsert to vector store...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = vectorStore.Upsert(ctx, []domain.Snippet{point})
	if err != nil {
		fmt.Printf("Error upserting to vector store: %v\n", err)
		return fallbackToFileStore(upsertInput)
	}

	return fmt.Sprintf("Successfully upserted content with ID: %s", id), nil
}

// fallbackToFileStore saves the content to a file when vector store operations fail
func fallbackToFileStore(input QdrantUpsertInput) (string, error) {
	fmt.Println("Falling back to file storage...")

	// Create a filename based on the current timestamp
	timestamp := time.Now().Format("20241201_120000")
	filename := fmt.Sprintf("vector_store_fallback_%s.txt", timestamp)

	// Prepare content including metadata if available
	var fileContent strings.Builder
	fileContent.WriteString(input.TextContent)

	if len(input.Metadata) > 0 {
		fileContent.WriteString("\n\n--- Metadata ---\n")
		for key, value := range input.Metadata {
			fileContent.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}

	// Create a file input for the create_file function
	createFileInput := CreateFileInput{
		Path:    filename,
		Content: fileContent.String(),
	}

	inputJSON, err := json.Marshal(createFileInput)
	if err != nil {
		return "", fmt.Errorf("failed to create fallback file (marshal error): %w", err)
	}

	return CreateFile(inputJSON)
}

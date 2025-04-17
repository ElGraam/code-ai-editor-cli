package infrastructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"

	"code-ai-editor/domain"
)

// FileToolRepository manages tool definitions and provides interfaces to interact with the BraveClient API.
// It holds a collection of tool definitions in the 'tools' slice and an instance of BraveClient in the 'braveClient'
// field, which is used to communicate with external services.
type FileToolRepository struct {
	tools       []domain.ToolDefinition
	braveClient *BraveClient
}

// NewFileToolRepository creates and returns a new FileToolRepository.
// It initializes the necessary file tool definitions used for reading,
// listing, and editing files. Additionally, if the creation of a Brave
// web search client is successful, it also adds a web search tool to the
// repository. The returned repository contains both the initialized tools
// and the Brave client (if available).
func NewFileToolRepository() *FileToolRepository {
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

	return &FileToolRepository{
		tools:       tools,
		braveClient: braveClient,
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
		return anthropic.NewToolResultBlock(id, err.Error(), true)
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
	}
}

// SearchWebInput defines the input parameters for the search_web tool.
// It contains a single field, Query, which represents the search query to execute.
type SearchWebInput struct {
	Query string `json:"query" jsonschema_description:"The search query to execute."`
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
	searchWebInput := SearchWebInput{}
	err := json.Unmarshal(input, &searchWebInput)
	if err != nil {
		return "", err
	}

	if searchWebInput.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	results, err := braveClient.Search(searchWebInput.Query)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}

	return string(resultJSON), nil
}

// ReadFileInput represents the input required to read a file from the working directory.
// It contains the relative path to the target file.
type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

// ReadFileDefinition returns a ToolDefinition for the "read_file" tool, which allows reading the contents
// of a specified file given its relative path. This tool should be used to inspect the contents of files,
// and not for directories. The function sets up the tool's name, description, input schema, and handler function.
func ReadFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
		InputSchema: GenerateSchema[ReadFileInput](),
		Function:    ReadFile,
	}
}

// ReadFile reads the contents of a file specified in the input JSON.
// The input should be a JSON object that can be unmarshaled into a ReadFileInput struct,
// which must contain the file path. It returns the file contents as a string,
// or an error if the file cannot be read or the input is invalid.
func ReadFile(input json.RawMessage) (string, error) {
	readFileInput := ReadFileInput{}
	err := json.Unmarshal(input, &readFileInput)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(readFileInput.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// ListFilesInput represents the input parameters for listing files in a directory.
// The Path field specifies an optional relative path to list files from.
// If Path is not provided, the current directory is used by default.
type ListFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from. Defaults to current directory if not provided."`
}

// ListFilesDefinition returns a ToolDefinition for listing files and directories at a specified path.
// If no path is provided, it lists files in the current directory. The tool uses ListFilesInput as its input schema
// and the ListFiles function as its implementation.
func ListFilesDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "list_files",
		Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
		InputSchema: GenerateSchema[ListFilesInput](),
		Function:    ListFiles,
	}
}

// ListFiles takes a JSON-encoded input specifying a directory path and returns a JSON-encoded
// list of all files and directories (with directories suffixed by a slash) within that path,
// traversing recursively. If no path is provided, it defaults to the current directory.
// The function returns the JSON string of file paths or an error if any occurs during processing.
//
// The expected input JSON should match the ListFilesInput struct, which must contain a "Path" field.
//
// Example input:
//
//	{"Path": "/some/directory"}
//
// Example output:
//
//	["file1.txt","subdir/","subdir/file2.txt"]
func ListFiles(input json.RawMessage) (string, error) {
	listFilesInput := ListFilesInput{}
	err := json.Unmarshal(input, &listFilesInput)
	if err != nil {
		return "", err
	}

	dir := "."
	if listFilesInput.Path != "" {
		dir = listFilesInput.Path
	}

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if relPath != "." {
			if info.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	result, err := json.Marshal(files)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// EditFileInput represents the input for editing a file.
// It contains the path to the file, the old string to be replaced,
// and the new string to replace the old string with.
type EditFileInput struct {
	Path   string `json:"path" jsonschema_description:"The path to the file"`
	OldStr string `json:"old_str" jsonschema_description:"Text to search for - must match exactly and must only have one match exactly"`
	NewStr string `json:"new_str" jsonschema_description:"Text to replace old_str with"`
}

// EditFileDefinition returns a domain.ToolDefinition for editing files.
//
// The tool allows replacing a string with another string in a given file.
// If the file does not exist, it will be created.
func EditFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name: "edit_file",
		Description: `Make edits to a text file.

			Replaces 'old_str' with 'new_str' in the given file. 'old_str' and 'new_str' MUST be different from each other.

			If the file specified with path doesn't exist, it will be created.
		`,
		InputSchema: GenerateSchema[EditFileInput](),
		Function:    EditFile,
	}
}

// EditFile edits an existing file or creates a new one based on the provided JSON input.
// The input must unmarshal into an EditFileInput struct containing:
//   - Path   (string): the file system path to read from or write to.
//   - OldStr (string): the substring to replace; if empty and the file does not exist, a new file is created.
//   - NewStr (string): the replacement string or the entire content for a new file.
//
// Returns "OK" on successful edit or creation. Returns an error if:
//   - the input parameters are invalid (empty Path or OldStr == NewStr),
//   - the file exists but OldStr is not found,
//   - or any file I/O operation fails.
func EditFile(input json.RawMessage) (string, error) {
	editFileInput := EditFileInput{}
	err := json.Unmarshal(input, &editFileInput)
	if err != nil {
		return "", err
	}

	if editFileInput.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	if editFileInput.OldStr == editFileInput.NewStr {
		return "", fmt.Errorf("old_str and new_str must be different")
	}

	_, statErr := os.Stat(editFileInput.Path)
	fileExists := !os.IsNotExist(statErr)

	if fileExists {
		if editFileInput.OldStr == "" {
			err = os.WriteFile(editFileInput.Path, []byte(editFileInput.NewStr), 0644)
			if err != nil {
				return "", fmt.Errorf("failed to overwrite file: %w", err)
			}
			return "OK (file overwritten)", nil
		} else {
			content, readErr := os.ReadFile(editFileInput.Path)
			if readErr != nil {
				return "", fmt.Errorf("failed to read existing file: %w", readErr)
			}
			oldContent := string(content)

			if !strings.Contains(oldContent, editFileInput.OldStr) {
				return "", fmt.Errorf("old_str not found in file")
			}

			newContent := strings.Replace(oldContent, editFileInput.OldStr, editFileInput.NewStr, -1)

			err = os.WriteFile(editFileInput.Path, []byte(newContent), 0644)
			if err != nil {
				return "", fmt.Errorf("failed to write file after replacement: %w", err)
			}
			return "OK (replaced)", nil
		}
	} else {
		if editFileInput.OldStr == "" {
			return createNewFile(editFileInput.Path, editFileInput.NewStr)
		} else {
			return "", fmt.Errorf("file not found and old_str is not empty, cannot replace")
		}
	}
}

// createNewFile creates a new file with the given file path and content.
// It also creates the necessary directories if they don't exist.
//
// Args:
//
//	filePath: The path to the new file.
//	content: The content of the new file.
//
// Returns:
//
//	A message indicating the file was successfully created, or an error if the file could not be created.
func createNewFile(filePath, content string) (string, error) {
	dir := path.Dir(filePath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	return fmt.Sprintf("Successfully created file %s", filePath), nil
}

// CreateFileInput represents the input parameters for creating a new file.
// It contains the path where the file should be created and the content to write to the file.
type CreateFileInput struct {
	Path    string `json:"path" jsonschema_description:"The path where the file should be created"`
	Content string `json:"content" jsonschema_description:"The content to write to the file"`
}

// CreateFileDefinition returns a ToolDefinition for the "create_file" tool, which allows creating
// a new file at the specified path with the given content. This tool is simpler to use than edit_file
// when you only need to create a new file without any replacements.
func CreateFileDefinition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "create_file",
		Description: "Create a new file with the specified content. If the file already exists, it will be overwritten.",
		InputSchema: GenerateSchema[CreateFileInput](),
		Function:    CreateFile,
	}
}

// CreateFile creates a new file based on the provided JSON input.
// The input must unmarshal into a CreateFileInput struct containing:
//   - Path    (string): the file system path where the file should be created.
//   - Content (string): the content to write to the file.
//
// Returns a success message on successful creation or an error if:
//   - the input parameters are invalid (empty Path),
//   - or any file I/O operation fails.
func CreateFile(input json.RawMessage) (string, error) {
	createFileInput := CreateFileInput{}
	err := json.Unmarshal(input, &createFileInput)
	if err != nil {
		return "", err
	}

	if createFileInput.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	return createNewFile(createFileInput.Path, createFileInput.Content)
}

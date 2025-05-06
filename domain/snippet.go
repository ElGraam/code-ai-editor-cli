package domain

// Snippet represents a chunk of code with its metadata and embedding.
type Snippet struct {
	ID        string            `json:"id"`                 // Unique identifier (e.g., UUID)
	Content   string            `json:"content"`            // The actual code content
	FilePath  string            `json:"file_path"`          // Path to the source file
	StartLine int               `json:"start_line"`         // Starting line number (1-based)
	EndLine   int               `json:"end_line"`           // Ending line number (1-based)
	Symbols   []string          `json:"symbols"`            // Symbols defined in this snippet (e.g., function names)
	Embedding Embedding         `json:"embedding"`          // Vector embedding of the content
	Metadata  map[string]string `json:"metadata,omitempty"` // Optional metadata
}

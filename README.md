# AI Code Editor CLI

An AI-powered command-line interface (CLI) chatbot that interacts with code files using the Anthropic Claude API. It can optionally leverage vector search for retrieving relevant code context.

## Overview

This project provides a CLI chatbot capable of understanding natural language commands to manipulate code files within your workspace. It leverages the Anthropic Claude API for its core AI capabilities and follows Domain-Driven Design (DDD) principles for a structured and maintainable codebase.

Optionally, it can index your Go codebase using embeddings (via OpenAI) and a vector database (Qdrant) to automatically retrieve relevant code snippets and inject them as context into the AI prompt, improving the AI's understanding of your codebase.

## Features

*   Conversational interaction via the Claude API.
*   **Code Context Retrieval (Optional):** Automatically finds relevant code snippets from an indexed codebase using vector search and adds them to the AI prompt.
*   Web search functionality through the Brave Search API (optional, requires configuration).
*   File system operations:
    *   Read file content.
    *   List files within a directory.
    *   Edit existing files.
    *   Create new files.
    *   Search the web (requires Brave API key).
*   **Codebase Indexing:** Indexes Go files in a specified directory for vector search.

## Architecture

The application employs a layered architecture inspired by DDD:

```
.
├── domain/
│   ├── agent.go            # Core domain logic, ReAct loop, context retrieval logic
│   ├── tool_definition.go  # Defines the ToolDefinition model
│   ├── embedding.go        # Interface for embedding clients
│   ├── vectorstore.go      # Interface for vector stores
│   ├── snippet.go          # Represents code snippets
│   └── code_parser.go      # Logic for parsing Go code into snippets
├── application/
│   ├── chatbot_service.go  # Implements chat use case
│   └── indexing_service.go # Implements indexing use case
├── infrastructure/
│   ├── anthropic_client.go # Wrapper for the Anthropic SDK
│   ├── brave_client.go     # Wrapper for the Brave Search API
│   ├── file_tools.go       # Implementation of file system tools
│   ├── embedding/
│   │   └── openai_embedding_client.go # OpenAI embedding client implementation
│   ├── vectorstore/
│   │   └── qdrant_client.go  # Qdrant vector store client implementation
│   └── memory/              # Memory-related implementations
└── main.go                 # Handles dependency injection, flags, and application startup
```

*   **Domain Layer:** Contains the core business logic, entities, value objects, and interfaces.
*   **Application Layer:** Orchestrates use cases (chat, indexing) by coordinating domain objects.
*   **Infrastructure Layer:** Deals with external concerns like API clients, vector databases, and file system interactions.

## Setup

1.  Obtain an API key from [Anthropic](https://www.anthropic.com/).
2.  (Optional, for Web Search) Obtain an API key from [Brave Search](https://brave.com/search/api/).
3.  (Optional, for Context Retrieval & Indexing) Obtain an API key from [OpenAI](https://openai.com/).
4.  (Optional, for Context Retrieval & Indexing) Set up a [Qdrant](https://qdrant.tech/documentation/quickstart/) instance (e.g., using Docker: `docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant`).
5.  Copy the example environment file `.env.sample` to `.env.local`:
    ```bash
    cp .env.sample .env.local
    ```
6.  Edit `.env.local` and add your API keys and Qdrant address:
    ```dotenv
    ANTHROPIC_API_KEY="your_anthropic_api_key_here"
    BRAVE_API_KEY="your_brave_api_key_here" # Optional
    OPENAI_API_KEY="your_openai_api_key_here"   # Optional, required for indexing/retrieval
    QDRANT_ADDR="localhost:6334"             # Optional, required for indexing/retrieval (gRPC port)
    QDRANT_COLLECTION_NAME="code_snippets"   # Optional, defaults to "code_snippets"
    ```
    *   If `OPENAI_API_KEY` is not provided, context retrieval will be disabled, but the chatbot will still function.

## Running the Application

### Indexing Your Codebase (Optional, One-time Step)

To enable context retrieval, first index your Go codebase **within the `workspace` directory**:

```bash
# Ensure the workspace directory exists if it doesn't
mkdir -p workspace

# Run indexing targeting the workspace directory
go run main.go --index
```

This process might take some time depending on the size of your codebase within the `workspace` directory and requires a valid `OPENAI_API_KEY` and a running Qdrant instance configured in `.env.local`. Only files within the workspace directory will be indexed.

### Starting the Chatbot

Execute the following command from the project root directory:

```bash
go run main.go
```

If you have indexed your codebase and configured the necessary environment variables, the chatbot will automatically retrieve relevant code snippets based on your queries and provide them as context to the AI.

## Available Tools

The chatbot can utilize the following tools. **Note:** When specifying file or directory paths for `read_file`, `list_files`, `edit_file`, and `create_file`, always provide paths relative to the `workspace/` directory (e.g., `workspace/my_folder/my_file.go`).

| Tool Name       | Description                                                     | Path Example                |
| :-------------- | :-------------------------------------------------------------- | :-------------------------- |
| `read_file`     | Reads the content of a specified file.                          | `workspace/src/main.go`     |
| `list_files`    | Lists the files and directories within a specified path.        | `workspace/src`             |
| `edit_file`     | Modifies the content of an existing file.                       | `workspace/data.txt`        |
| `create_file`   | Creates a new file with the specified content.                  | `workspace/new_file.txt`    |
| `search_web`    | Performs a web search using the Brave Search API (if configured). | N/A                         |
| `qdrant_search` | Searches for relevant information in the Qdrant vector store using a query string that will be embedded. Requires `OPENAI_API_KEY` and Qdrant. | N/A                         |
| `qdrant_upsert` | Upserts (embeds and then inserts or updates) information into the Qdrant vector store. Requires `OPENAI_API_KEY` and Qdrant. | N/A                         |

## Development

### Adding New Tools

To extend the chatbot's capabilities with a new tool:

1.  Define the tool's structure and implement its logic within the `infrastructure` layer (e.g., in `infrastructure/file_tools.go` or a new file).
2.  Register the new tool in the `FileToolRepository.NewFileToolRepository` method located in `infrastructure/file_tools.go`.
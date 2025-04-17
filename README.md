# AI Code Editor CLI

An AI-powered command-line interface (CLI) chatbot that interacts with code files using the Anthropic Claude API.

## Overview

This project provides a CLI chatbot capable of understanding natural language commands to manipulate code files within your workspace. It leverages the Anthropic Claude API for its core AI capabilities and follows Domain-Driven Design (DDD) principles for a structured and maintainable codebase.

## Features

*   Conversational interaction via the Claude API.
*   Web search functionality through the Brave Search API (optional, requires configuration).
*   File system operations:
    *   Read file content.
    *   List files within a directory.
    *   Edit existing files.
    *   Search the web (requires Brave API key).

## Architecture

The application employs a layered architecture inspired by DDD:

```
.
├── domain/
│   ├── agent.go            # Core domain logic, entities, and tool definitions
│   └── tool_definition.go  # Defines the ToolDefinition model
├── application/
│   └── chatbot_service.go  # Implements use cases, orchestrating domain logic
├── infrastructure/
│   ├── anthropic_client.go # Wrapper for the Anthropic SDK
│   ├── brave_client.go     # Wrapper for the Brave Search API
│   └── file_tools.go       # Implementation of file system tools
└── main.go                 # Handles dependency injection and application startup
```

*   **Domain Layer:** Contains the core business logic, entities, and value objects.
*   **Application Layer:** Orchestrates use cases by coordinating domain objects.
*   **Infrastructure Layer:** Deals with external concerns like API clients and file system interactions.

## Setup

1.  Obtain an API key from [Anthropic](https://www.anthropic.com/).
2.  (Optional) Obtain an API key from [Brave Search](https://brave.com/search/api/).
3.  Copy the example environment file `.env.sample` to `.env.local`:
    ```bash
    cp .env.sample .env.local
    ```
4.  Edit `.env.local` and add your API keys:
    ```dotenv
    ANTHROPIC_API_KEY="your_anthropic_api_key_here"
    BRAVE_API_KEY="your_brave_api_key_here" # Optional
    ```

## Running the Application

Execute the following command from the project root directory:

```bash
go run main.go
```

## Available Tools

The chatbot can utilize the following tools:

| Tool Name    | Description                                                     |
| :----------- | :-------------------------------------------------------------- |
| `read_file`  | Reads the content of a specified file.                          |
| `list_files` | Lists the files and directories within a specified path.        |
| `edit_file`  | Modifies the content of an existing file.                       |
| `search_web` | Performs a web search using the Brave Search API (if configured). |

## Development

### Adding New Tools

To extend the chatbot's capabilities with a new tool:

1.  Define the tool's structure and implement its logic within the `infrastructure` layer (e.g., in `infrastructure/file_tools.go` or a new file).
2.  Register the new tool in the `FileToolRepository.NewFileToolRepository` method located in `infrastructure/file_tools.go`.
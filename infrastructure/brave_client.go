package infrastructure

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// BraveClient is a client for interacting with the Brave Search API.
// It encapsulates the API key, HTTP client, and base URL for making requests.
type BraveClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// BraveSearchResponse represents the structure of the JSON response from the Brave Search API.
// It includes web search results, with each result containing a title, URL, and description.
type BraveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// NewBraveClient creates a new BraveClient instance.
// It retrieves the Brave API key from the BRAVE_API_KEY environment variable.
// If the environment variable is not set, it returns an error.
// Otherwise, it initializes and returns a pointer to a BraveClient
// configured with the API key, a default HTTP client, and the base URL
// for the Brave Search API.
func NewBraveClient() (*BraveClient, error) {
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY is not set")
	}

	return &BraveClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.search.brave.com/res/v1/web/search",
	}, nil
}

// Search performs a search query against the Brave Search API.
//
// It constructs a GET request to the Brave Search API endpoint with the given query,
// sets the necessary headers (Accept and X-Subscription-Token), and executes the request.
// If the request is successful (status code 200), it parses the JSON response into a
// BraveSearchResponse struct and returns it. If any error occurs during the process,
// such as request creation, API request failure, or response parsing, it returns an error
// with a descriptive message.
//
// Args:
//
//	query: The search query string.
//
// Returns:
//
//	A pointer to a BraveSearchResponse struct containing the search results, or an error if the search failed.
func (b *BraveClient) Search(query string) (*BraveSearchResponse, error) {
	params := url.Values{}
	params.Add("q", query)

	req, err := http.NewRequest("GET", b.baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-Subscription-Token", b.apiKey)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status code %d): %s", resp.StatusCode, string(body))
	}

	var searchResp BraveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &searchResp, nil
}

// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// Package search queries the skills.sh registry for available skills.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DefaultRegistryURL is the base URL for the skills.sh registry.
const DefaultRegistryURL = "https://skills.sh"

const defaultLimit = 10

// Result is a single skill returned from the registry search.
type Result struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Installs int    `json:"installs"`
}

// Options configures a search run.
type Options struct {
	// Query is the search string sent to the registry.
	Query string

	// Limit is the maximum number of results to return. When zero the default
	// of 20 is used.
	Limit int

	// RegistryURL overrides the default registry endpoint. When empty
	// DefaultRegistryURL is used.
	RegistryURL string
}

// apiResponse mirrors the skills.sh /api/search JSON envelope.
type apiResponse struct {
	Skills []struct {
		Name     string `json:"name"`
		Source   string `json:"source"`
		Installs int    `json:"installs"`
	} `json:"skills"`
}

// Searcher queries the skills.sh registry.
type Searcher struct{}

// New returns a Searcher ready to query the registry.
func New() *Searcher { return &Searcher{} }

// Run queries the registry with opts and returns the matching skills.
func (s *Searcher) Run(ctx context.Context, opts Options) ([]Result, error) {
	registryURL := opts.RegistryURL
	if registryURL == "" {
		registryURL = DefaultRegistryURL
	}

	limit := opts.Limit
	if limit == 0 {
		limit = defaultLimit
	}

	endpoint, err := url.Parse(registryURL + "/api/search")
	if err != nil {
		return nil, fmt.Errorf("parse registry URL: %w", err)
	}

	q := endpoint.Query()
	q.Set("q", opts.Query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request: unexpected status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]Result, len(apiResp.Skills))
	for i, sk := range apiResp.Skills {
		results[i] = Result{
			Name:     sk.Name,
			Source:   sk.Source,
			Installs: sk.Installs,
		}
	}

	return results, nil
}

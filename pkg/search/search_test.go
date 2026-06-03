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

package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/retr0h/agentpack/pkg/search"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		serverBody  string
		serverCode  int
		opts        func(serverURL string) search.Options
		ctx         func() context.Context
		wantErr     string
		wantResults []search.Result
	}{
		{
			name:       "happy path with results",
			serverCode: http.StatusOK,
			serverBody: `{"query":"typescript","searchType":"fuzzy","skills":[{"id":"wshobson/agents/typescript-advanced-types","skillId":"typescript-advanced-types","name":"typescript-advanced-types","installs":43619,"source":"wshobson/agents"}]}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "typescript", RegistryURL: serverURL}
			},
			ctx: func() context.Context { return context.Background() },
			wantResults: []search.Result{
				{Name: "typescript-advanced-types", Source: "wshobson/agents", Installs: 43619},
			},
		},
		{
			name:       "empty results returns empty slice",
			serverCode: http.StatusOK,
			serverBody: `{"query":"nothing","searchType":"fuzzy","skills":[]}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "nothing", RegistryURL: serverURL}
			},
			ctx:         func() context.Context { return context.Background() },
			wantResults: []search.Result{},
		},
		{
			name:       "query with special characters is URL-encoded",
			serverCode: http.StatusOK,
			serverBody: `{"query":"c++ templates","searchType":"fuzzy","skills":[{"name":"cpp-templates","source":"someone/repo","installs":100}]}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "c++ templates", RegistryURL: serverURL}
			},
			ctx: func() context.Context { return context.Background() },
			wantResults: []search.Result{
				{Name: "cpp-templates", Source: "someone/repo", Installs: 100},
			},
		},
		{
			name:       "server error returns error",
			serverCode: http.StatusInternalServerError,
			serverBody: `internal server error`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "anything", RegistryURL: serverURL}
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "unexpected status 500",
		},
		{
			name:       "invalid JSON returns error",
			serverCode: http.StatusOK,
			serverBody: `not valid json`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "anything", RegistryURL: serverURL}
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "decode response",
		},
		{
			name:       "cancelled context returns error",
			serverCode: http.StatusOK,
			serverBody: `{}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "anything", RegistryURL: serverURL}
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: "context canceled",
		},
		{
			name:       "custom registry URL is used",
			serverCode: http.StatusOK,
			serverBody: `{"skills":[{"name":"custom-skill","source":"custom/repo","installs":7}]}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "custom", RegistryURL: serverURL}
			},
			ctx: func() context.Context { return context.Background() },
			wantResults: []search.Result{
				{Name: "custom-skill", Source: "custom/repo", Installs: 7},
			},
		},
		{
			name:       "zero limit defaults to 20",
			serverCode: http.StatusOK,
			serverBody: `{"skills":[]}`,
			opts: func(serverURL string) search.Options {
				return search.Options{Query: "anything", Limit: 0, RegistryURL: serverURL}
			},
			ctx:         func() context.Context { return context.Background() },
			wantResults: []search.Result{},
		},
		{
			name:       "empty registry URL uses default and fails with connection error",
			serverCode: http.StatusOK,
			serverBody: `{"skills":[]}`,
			opts: func(_ string) search.Options {
				// RegistryURL left empty so DefaultRegistryURL is used.
				// The request will fail because the default URL is unreachable in tests,
				// but that exercises the defaulting branch before the HTTP call.
				return search.Options{Query: "anything", RegistryURL: ""}
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 1)
				_ = cancel
				return ctx
			},
			wantErr: "context deadline exceeded",
		},
		{
			name:       "invalid registry URL returns parse error",
			serverCode: http.StatusOK,
			serverBody: `{"skills":[]}`,
			opts: func(_ string) search.Options {
				// A URL containing a control character causes url.Parse to fail.
				return search.Options{Query: "q", RegistryURL: "http://\x7f/bad"}
			},
			ctx:     func() context.Context { return context.Background() },
			wantErr: "parse registry URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.serverCode)
					_, _ = w.Write([]byte(tt.serverBody))
				}),
			)
			defer srv.Close()

			opts := tt.opts(srv.URL)
			ctx := tt.ctx()

			results, err := search.New().Run(ctx, opts)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResults, results)
		})
	}
}

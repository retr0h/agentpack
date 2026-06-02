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

package testutil

import (
	"context"
	"sync/atomic"
	"time"
)

// CancelAfterN is a context.Context whose Err() returns nil for the first n
// calls and then cancels the context on the (n+1)th call and all subsequent
// calls. Pass n=0 to cancel on every call (including the first).
//
// Done() always returns nil so tests never block waiting on the channel;
// cancellation is detected only through the explicit Err() poll that
// production loops perform.
type CancelAfterN struct {
	context.Context
	n      int32
	calls  atomic.Int32
	cancel context.CancelFunc
}

// NewCancelAfterN creates a CancelAfterN that returns nil from Err() for the
// first n calls and cancels on the (n+1)th call.
func NewCancelAfterN(n int) *CancelAfterN {
	ctx, cancel := context.WithCancel(context.Background())
	return &CancelAfterN{Context: ctx, n: int32(n), cancel: cancel}
}

// Deadline implements context.Context.
func (c *CancelAfterN) Deadline() (time.Time, bool) { return time.Time{}, false }

// Done implements context.Context. Always returns nil so tests never block.
func (c *CancelAfterN) Done() <-chan struct{} { return nil }

// Value implements context.Context.
func (c *CancelAfterN) Value(_ any) any { return nil }

// Err implements context.Context. Returns nil for the first n calls, then
// cancels the underlying context and returns context.Canceled thereafter.
func (c *CancelAfterN) Err() error {
	if c.calls.Add(1) > c.n {
		c.cancel()
	}
	return c.Context.Err()
}

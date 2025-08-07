package middleware

import (
	"context"
	"net/http"
)

func WithContext(backgroundCtx context.Context, h http.HandlerFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// Create a combined context that cancels when either:
		// 1. The request context is cancelled (client disconnect, timeout)
		// 2. The background context is cancelled (manager shutdown)
		combinedCtx, cancel := context.WithCancel(backgroundCtx)
		defer cancel()

		// Start a goroutine to cancel combined context if request context is done
		go func() {
			select {
			case <-req.Context().Done():
				cancel()
			case <-combinedCtx.Done():
				// Combined context already cancelled, nothing to do
			}
		}()

		req = req.WithContext(combinedCtx)
		h(res, req)
	}
}

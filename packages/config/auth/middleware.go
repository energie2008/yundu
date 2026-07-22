package auth

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

func SignatureMiddleware(secret string, nonceStore NonceStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				next.ServeHTTP(w, r)
				return
			}

			timestampStr := r.Header.Get(HeaderTimestamp)
			nonce := r.Header.Get(HeaderNonce)
			signature := r.Header.Get(HeaderSignature)

			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			if err := VerifySignature(r.Method, r.URL.Path, string(bodyBytes), timestampStr, nonce, signature, secret); err != nil {
				if sigErr, ok := err.(*SignatureError); ok {
					http.Error(w, sigErr.Message, http.StatusUnauthorized)
					w.Header().Set("X-Error-Code", sigErr.Code)
					return
				}
				http.Error(w, "signature verification failed", http.StatusUnauthorized)
				return
			}

			if nonceStore != nil {
				ok, err := nonceStore.CheckAndStore(r.Context(), nonce, 10*time.Minute)
				if err != nil || !ok {
					http.Error(w, "REPLAY_DETECTED", http.StatusConflict)
					w.Header().Set("X-Error-Code", "REPLAY_DETECTED")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

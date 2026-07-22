package auth

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func SignatureGinMiddleware(secret string, nonceStore NonceStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}

		timestampStr := c.GetHeader(HeaderTimestamp)
		nonce := c.GetHeader(HeaderNonce)
		signature := c.GetHeader(HeaderSignature)

		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		if err := VerifySignature(c.Request.Method, c.Request.URL.Path, string(bodyBytes), timestampStr, nonce, signature, secret); err != nil {
			if sigErr, ok := err.(*SignatureError); ok {
				c.Header("X-Error-Code", sigErr.Code)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": sigErr.Message,
					"code":  sigErr.Code,
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "signature verification failed",
			})
			return
		}

		if nonceStore != nil {
			ok, err := nonceStore.CheckAndStore(c.Request.Context(), nonce, 10*time.Minute)
			if err != nil || !ok {
				c.Header("X-Error-Code", "REPLAY_DETECTED")
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{
					"error": "replay detected",
					"code":  "REPLAY_DETECTED",
				})
				return
			}
		}

		c.Next()
	}
}

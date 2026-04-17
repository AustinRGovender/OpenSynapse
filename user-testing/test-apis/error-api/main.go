package main

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	requestCount int
	rateLimited  bool
	mu           sync.Mutex
)

func main() {
	router := gin.Default()
	rand.Seed(time.Now().UnixNano())

	// Reset rate limit every 10 seconds
	go func() {
		for {
			time.Sleep(10 * time.Second)
			mu.Lock()
			requestCount = 0
			rateLimited = false
			mu.Unlock()
		}
	}()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "Error Generator API"})
	})

	// Random errors
	router.GET("/random-error", func(c *gin.Context) {
		codes := []int{400, 401, 403, 404, 429, 500, 502, 503, 504}
		statusCode := codes[rand.Intn(len(codes))]
		c.JSON(statusCode, gin.H{
			"error":       fmt.Sprintf("Random error %d", statusCode),
			"status_code": statusCode,
		})
	})

	// Rate limiting
	router.GET("/rate-limit", func(c *gin.Context) {
		mu.Lock()
		requestCount++
		if requestCount > 100 {
			rateLimited = true
		}
		mu.Unlock()

		if rateLimited {
			c.JSON(429, gin.H{"error": "Rate limit exceeded"})
			return
		}
		c.JSON(200, gin.H{"message": "OK", "requests": requestCount})
	})

	// Progressive degradation
	router.GET("/degradation", func(c *gin.Context) {
		mu.Lock()
		count := requestCount
		requestCount++
		mu.Unlock()

		// Response time increases with request count
		delay := time.Duration(count*10) * time.Millisecond
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		time.Sleep(delay)

		// Error rate increases
		if count > 50 && rand.Float32() < 0.3 {
			c.JSON(500, gin.H{"error": "Service degraded"})
			return
		}

		c.JSON(200, gin.H{
			"message":     "OK",
			"request_num": count,
			"delay_ms":    delay.Milliseconds(),
		})
	})

	fmt.Println("❌ Error Generator API starting on port 9004")
	if err := router.Run(":9004"); err != nil {
		log.Fatal("Failed to start Error Generator API:", err)
	}
}

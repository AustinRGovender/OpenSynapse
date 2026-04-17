package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "Echo API",
			"port":    9001,
		})
	})

	// Echo request body
	router.POST("/echo", func(c *gin.Context) {
		var body map[string]interface{}
		if err := c.BindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "Invalid JSON"})
			return
		}

		c.JSON(200, gin.H{
			"echo":      body,
			"timestamp": time.Now().Unix(),
			"method":    c.Request.Method,
			"headers":   c.Request.Header,
		})
	})

	// Delayed response
	router.GET("/delay/:ms", func(c *gin.Context) {
		ms, err := strconv.Atoi(c.Param("ms"))
		if err != nil || ms < 0 {
			c.JSON(400, gin.H{"error": "Invalid delay parameter"})
			return
		}

		if ms > 10000 {
			c.JSON(400, gin.H{"error": "Delay too long (max 10000ms)"})
			return
		}

		time.Sleep(time.Duration(ms) * time.Millisecond)

		c.JSON(200, gin.H{
			"delayed_ms": ms,
			"timestamp":  time.Now().Unix(),
		})
	})

	// Return specific status code
	router.GET("/status/:code", func(c *gin.Context) {
		code, err := strconv.Atoi(c.Param("code"))
		if err != nil || code < 100 || code > 599 {
			c.JSON(400, gin.H{"error": "Invalid status code"})
			return
		}

		c.JSON(code, gin.H{
			"status_code": code,
			"timestamp":   time.Now().Unix(),
		})
	})

	fmt.Println("🔊 Echo API starting on port 9001")
	fmt.Println("📍 Endpoints:")
	fmt.Println("   GET  /health")
	fmt.Println("   POST /echo")
	fmt.Println("   GET  /delay/:ms")
	fmt.Println("   GET  /status/:code")

	if err := router.Run(":9001"); err != nil {
		log.Fatal("Failed to start Echo API:", err)
	}
}

package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	rand.Seed(time.Now().UnixNano())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "Slow API"})
	})

	// Slow response
	router.GET("/slow/:seconds", func(c *gin.Context) {
		seconds, err := strconv.Atoi(c.Param("seconds"))
		if err != nil || seconds < 0 || seconds > 30 {
			c.JSON(400, gin.H{"error": "Invalid seconds (0-30)"})
			return
		}

		time.Sleep(time.Duration(seconds) * time.Second)
		c.JSON(200, gin.H{
			"delayed_seconds": seconds,
			"timestamp":       time.Now().Unix(),
		})
	})

	// Timeout simulation
	router.GET("/timeout", func(c *gin.Context) {
		time.Sleep(60 * time.Second)
		c.JSON(200, gin.H{"message": "This should timeout"})
	})

	// Flaky endpoint
	router.GET("/flaky", func(c *gin.Context) {
		if rand.Float32() < 0.5 {
			time.Sleep(time.Duration(rand.Intn(5)) * time.Second)
			c.JSON(500, gin.H{"error": "Random failure"})
		} else {
			c.JSON(200, gin.H{"message": "Success"})
		}
	})

	fmt.Println("🐌 Slow API starting on port 9003")
	if err := router.Run(":9003"); err != nil {
		log.Fatal("Failed to start Slow API:", err)
	}
}

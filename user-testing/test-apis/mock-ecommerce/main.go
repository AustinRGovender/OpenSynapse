package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}

type Order struct {
	ID         string   `json:"id"`
	ProductIDs []string `json:"product_ids"`
	Total      float64  `json:"total"`
	Status     string   `json:"status"`
}

var (
	products = make(map[string]Product)
	orders   = make(map[string]Order)
	mu       sync.RWMutex
)

func init() {
	// Sample products
	sampleProducts := []Product{
		{ID: "p1", Name: "Laptop", Price: 999.99, Stock: 10},
		{ID: "p2", Name: "Mouse", Price: 29.99, Stock: 50},
		{ID: "p3", Name: "Keyboard", Price: 79.99, Stock: 30},
		{ID: "p4", Name: "Monitor", Price: 299.99, Stock: 15},
	}
	for _, p := range sampleProducts {
		products[p.ID] = p
	}
}

func main() {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "Mock E-commerce API"})
	})

	// Products
	router.GET("/products", func(c *gin.Context) {
		mu.RLock()
		defer mu.RUnlock()
		productList := make([]Product, 0, len(products))
		for _, p := range products {
			productList = append(productList, p)
		}
		c.JSON(200, productList)
	})

	router.GET("/products/:id", func(c *gin.Context) {
		mu.RLock()
		defer mu.RUnlock()
		id := c.Param("id")
		if product, ok := products[id]; ok {
			c.JSON(200, product)
		} else {
			c.JSON(404, gin.H{"error": "Product not found"})
		}
	})

	// Orders
	router.POST("/orders", func(c *gin.Context) {
		var req struct {
			ProductIDs []string `json:"product_ids"`
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		mu.Lock()
		defer mu.Unlock()

		total := 0.0
		for _, pid := range req.ProductIDs {
			if p, ok := products[pid]; ok {
				total += p.Price
			}
		}

		order := Order{
			ID:         uuid.New().String(),
			ProductIDs: req.ProductIDs,
			Total:      total,
			Status:     "pending",
		}
		orders[order.ID] = order

		c.JSON(201, order)
	})

	router.GET("/orders/:id", func(c *gin.Context) {
		mu.RLock()
		defer mu.RUnlock()
		id := c.Param("id")
		if order, ok := orders[id]; ok {
			c.JSON(200, order)
		} else {
			c.JSON(404, gin.H{"error": "Order not found"})
		}
	})

	fmt.Println("🛒 Mock E-commerce API starting on port 9002")
	if err := router.Run(":9002"); err != nil {
		log.Fatal("Failed to start Mock E-commerce API:", err)
	}
}

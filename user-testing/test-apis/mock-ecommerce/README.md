# Mock E-commerce API

A sample e-commerce API for performance testing with realistic CRUD operations.

## Endpoints

### Health Check
```bash
GET http://localhost:9002/health
```

### Products

**List All Products**
```bash
GET http://localhost:9002/products
```

**Get Product by ID**
```bash
GET http://localhost:9002/products/p1
```

### Orders

**Create Order**
```bash
POST http://localhost:9002/orders
Content-Type: application/json

{
  "product_ids": ["p1", "p2"]
}
```

**Get Order by ID**
```bash
GET http://localhost:9002/orders/{order_id}
```

## Sample Products

- `p1`: Laptop - $999.99 (10 in stock)
- `p2`: Mouse - $29.99 (50 in stock)
- `p3`: Keyboard - $79.99 (30 in stock)
- `p4`: Monitor - $299.99 (15 in stock)

## Running

```bash
# Standalone
go run main.go

# Docker
docker build -t mock-ecommerce .
docker run -p 9002:9002 mock-ecommerce
```

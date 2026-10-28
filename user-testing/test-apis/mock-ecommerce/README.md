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

**Latest order**
```bash
GET http://localhost:9002/orders/latest
```

### OpenAPI document

```bash
GET http://localhost:9002/openapi.yaml
```

## Storefront (for the crawler)

`/shop` is a small HTML shop for trying the Crawler. Each page tests one crawler
feature. Open `http://localhost:9002/shop` in a browser to look around.

| Page | What it tests |
| --- | --- |
| `/shop`, `/shop/products`, `/shop/products?page=2`, `/shop/products/{id}`, `/shop/cart`, `/shop/about` | Plain links: every engine finds these |
| `/shop/app` | Builds its links to `/shop/deals/{id}` with JavaScript, so only a browser engine (Rod) finds them |
| `/shop/login` (field `username`) | Form login: `demo` / `demo-pass` |
| `/shop/login-email` (field `email`) | Form login: `demo@example.com` / `demo-pass` |
| `/shop/account`, `/shop/account/orders`, `/shop/account/settings` | Need a signed-in session; without one they redirect to the login page |
| `/shop/staff`, `/shop/staff/reports` | HTTP Basic auth: `demo` / `demo-pass` |
| `/shop/logout`, `/shop/account/delete` | Traps: they end the session. The crawler's default skip list (`/logout`, `/delete`) avoids them |
| `https://example.com/partner` (linked from the home page) | A link to another site; not followed while "same origin only" is on |

Adding to the cart and checking out (`POST /shop/cart`, `POST /shop/checkout`) work in a
browser and place real orders in the API above, but the crawlers do not submit forms
other than the login form.

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

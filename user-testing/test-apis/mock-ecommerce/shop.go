package main

// A small HTML storefront under /shop for testing OpenSynapse's crawler.
// It is built so each crawler feature has something to find (or to avoid):
//
//   - linked pages, pagination and per-product pages (link following);
//   - forms that POST (add to cart, checkout, log in);
//   - a login form at /shop/login (field "username") and another at
//     /shop/login-email (field "email"); demo / demo-pass, or
//     demo@example.com / demo-pass. /shop/account needs a signed-in session;
//   - /shop/staff behind HTTP Basic auth (demo / demo-pass);
//   - /shop/app, whose deal links only exist after JavaScript runs and which
//     calls the JSON API (GET /products) from the browser: only a browser
//     crawler (Rod) finds them;
//   - traps a crawler should skip: /shop/logout and /shop/account/delete
//     (the default blocklist) and a link to another site (same-origin rule).
//
// What each engine is expected to find is listed in README.md.

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const sessionCookie = "shop_session"

type shopSession struct {
	User   string
	Cart   []string
	Orders []string
}

var (
	sessions   = map[string]*shopSession{}
	sessionsMu sync.Mutex
)

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// session returns the visitor's session, creating one (and its cookie) if needed.
func session(c *gin.Context) *shopSession {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if tok, err := c.Cookie(sessionCookie); err == nil {
		if s, ok := sessions[tok]; ok {
			return s
		}
	}
	tok := newToken()
	s := &shopSession{}
	sessions[tok] = s
	c.SetCookie(sessionCookie, tok, 3600, "/shop", "", false, true)
	return s
}

var shopTemplates = template.Must(template.New("layout").Parse(`{{define "layout"}}<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>{{.Title}} · Synapse Shop</title>
<style>body{font-family:system-ui,sans-serif;max-width:46rem;margin:2rem auto;padding:0 1rem;line-height:1.5}
nav a{margin-right:.8rem}form{margin:1rem 0}label{display:block;margin:.4rem 0}.err{color:crimson}</style></head>
<body>
<nav>
  <a href="/shop">Home</a>
  <a href="/shop/products">Products</a>
  <a href="/shop/cart">Cart{{if .CartCount}} ({{.CartCount}}){{end}}</a>
  <a href="/shop/app">Deals</a>
  <a href="/shop/about">About</a>
  {{if .User}}<a href="/shop/account">My account</a> <a href="/shop/logout">Log out</a>{{else}}<a href="/shop/login">Log in</a>{{end}}
</nav>
<h1>{{.Title}}</h1>
{{template "body" .}}
</body></html>{{end}}`))

func page(body string) *template.Template {
	t := template.Must(shopTemplates.Clone())
	return template.Must(t.New("body").Parse(body))
}

var (
	homePage = page(`<p>Welcome to the Synapse Shop, a test site for crawling.</p>
<h2>Featured</h2>
<ul>{{range .Products}}<li><a href="/shop/products/{{.ID}}">{{.Name}}</a></li>{{end}}</ul>
<p><a href="https://example.com/partner">Our partner store</a> (another site)</p>
<p><a href="/shop/staff">Staff area</a> (HTTP Basic auth)</p>
<p><a href="/shop/account/delete">Delete my account</a></p>`)

	productsPage = page(`<ul>{{range .Products}}<li><a href="/shop/products/{{.ID}}">{{.Name}}</a> — ${{printf "%.2f" .Price}}</li>{{end}}</ul>
{{if .NextPage}}<p><a href="/shop/products?page={{.NextPage}}">Next page</a></p>{{else}}<p><a href="/shop/products">First page</a></p>{{end}}`)

	productPage = page(`<p>{{.Product.Name}} — ${{printf "%.2f" .Product.Price}} ({{.Product.Stock}} in stock)</p>
<form method="post" action="/shop/cart">
  <input type="hidden" name="product_id" value="{{.Product.ID}}">
  <label>Quantity <input type="number" name="quantity" value="1" min="1"></label>
  <button type="submit">Add to cart</button>
</form>
<p><a href="/shop/products">Back to products</a></p>`)

	cartPage = page(`{{if .Items}}<ul>{{range .Items}}<li>{{.Name}} — ${{printf "%.2f" .Price}}</li>{{end}}</ul>
<form method="post" action="/shop/checkout"><button type="submit">Place order</button></form>
{{else}}<p>Your cart is empty.</p>{{end}}
<p><a href="/shop/products">Continue shopping</a></p>`)

	orderPage = page(`{{if .Order}}<p>Order {{.Order.ID}}: {{len .Order.ProductIDs}} item(s), ${{printf "%.2f" .Order.Total}}, {{.Order.Status}}.</p>
<p><a href="/orders/{{.Order.ID}}">As JSON</a></p>{{else}}<p>No such order.</p>{{end}}
<p><a href="/shop">Home</a></p>`)

	loginPage = page(`{{if .Error}}<p class="err">{{.Error}}</p>{{end}}
{{if .User}}<p>You are signed in as {{.User}}. <a href="/shop/account">Go to your account</a>.</p>{{end}}
<form method="post" action="{{.Action}}">
  {{if .EmailField}}<label>Email <input type="email" name="email" autocomplete="username"></label>
  {{else}}<label>Username <input name="username" autocomplete="username"></label>{{end}}
  <label>Password <input type="password" name="password" autocomplete="current-password"></label>
  <button type="submit">Log in</button>
</form>
<p>Test account: {{if .EmailField}}demo@example.com{{else}}demo{{end}} / demo-pass.
{{if .EmailField}}<a href="/shop/login">Log in with a username</a>{{else}}<a href="/shop/login-email">Log in with an email</a>{{end}}</p>`)

	accountPage = page(`<p>Signed in as {{.User}}.</p>
<ul><li><a href="/shop/account/orders">My orders</a></li><li><a href="/shop/account/settings">Settings</a></li></ul>`)

	accountOrdersPage = page(`{{if .Orders}}<ul>{{range .Orders}}<li><a href="/shop/orders/{{.}}">{{.}}</a></li>{{end}}</ul>{{else}}<p>No orders yet. <a href="/shop/products">Start shopping</a>.</p>{{end}}`)

	settingsPage = page(`<form method="post" action="/shop/account/settings">
  <label>Display name <input name="display_name" value="{{.User}}"></label>
  <button type="submit">Save</button>
</form>`)

	aboutPage = page(`<p>The Synapse Shop exists to be crawled. Its pages, forms and traps are described in the test APIs' README.</p>`)

	staffPage = page(`<p>Staff only.</p><ul><li><a href="/shop/staff/reports">Sales reports</a></li></ul>`)

	reportsPage = page(`<p>Orders today: {{.OrderCount}}.</p><p><a href="/shop/staff">Back</a></p>`)

	deletePage = page(`<p>Your account was deleted. (A crawler that follows every link would do this; OpenSynapse skips "/delete" by default.)</p>`)

	// The deal links are built in the browser from GET /products, so only a
	// crawler that runs JavaScript sees them.
	appPage = page(`<p id="status">Loading deals…</p>
<ul id="deals"></ul>
<script>
fetch('/products').then(function (r) { return r.json() }).then(function (items) {
  items.sort(function (a, b) { return a.id < b.id ? -1 : 1 })
  var list = document.getElementById('deals')
  items.forEach(function (p) {
    var li = document.createElement('li')
    var a = document.createElement('a')
    a.href = '/shop/deals/' + p.id
    a.textContent = 'Deal: ' + p.name
    li.appendChild(a)
    list.appendChild(li)
  })
  document.getElementById('status').textContent = items.length + ' deals today'
})
</script>`)

	dealPage = page(`{{if .Product}}<p>Today only: {{.Product.Name}} at ${{printf "%.2f" .DealPrice}}.</p>
<p><a href="/shop/products/{{.Product.ID}}">See the product</a></p>{{else}}<p>No such deal.</p>{{end}}`)
)

type pageData struct {
	Title      string
	User       string
	CartCount  int
	Products   []Product
	Product    *Product
	NextPage   int
	Items      []Product
	Order      *Order
	Orders     []string
	Error      string
	Action     string
	EmailField bool
	OrderCount int
	DealPrice  float64
}

func render(c *gin.Context, status int, t *template.Template, d pageData) {
	s := session(c)
	d.User = s.User
	d.CartCount = len(s.Cart)
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(c.Writer, "layout", d); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
}

func sortedProducts() []Product {
	mu.RLock()
	defer mu.RUnlock()
	list := make([]Product, 0, len(products))
	for _, p := range products {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func productByID(id string) *Product {
	mu.RLock()
	defer mu.RUnlock()
	if p, ok := products[id]; ok {
		return &p
	}
	return nil
}

func requireLogin(c *gin.Context) bool {
	if session(c).User == "" {
		c.Redirect(http.StatusFound, "/shop/login")
		return false
	}
	return true
}

func registerShop(router *gin.Engine) {
	shop := router.Group("/shop")

	shop.GET("", func(c *gin.Context) {
		render(c, 200, homePage, pageData{Title: "Synapse Shop", Products: sortedProducts()[:2]})
	})
	shop.GET("/products", func(c *gin.Context) {
		all := sortedProducts()
		if c.Query("page") == "2" {
			render(c, 200, productsPage, pageData{Title: "Products (page 2)", Products: all[2:]})
			return
		}
		render(c, 200, productsPage, pageData{Title: "Products", Products: all[:2], NextPage: 2})
	})
	shop.GET("/products/:id", func(c *gin.Context) {
		p := productByID(c.Param("id"))
		if p == nil {
			render(c, 404, aboutPage, pageData{Title: "Product not found"})
			return
		}
		render(c, 200, productPage, pageData{Title: p.Name, Product: p})
	})
	shop.POST("/cart", func(c *gin.Context) {
		if p := productByID(c.PostForm("product_id")); p != nil {
			s := session(c)
			sessionsMu.Lock()
			s.Cart = append(s.Cart, p.ID)
			sessionsMu.Unlock()
		}
		c.Redirect(http.StatusSeeOther, "/shop/cart")
	})
	shop.GET("/cart", func(c *gin.Context) {
		s := session(c)
		var items []Product
		for _, id := range s.Cart {
			if p := productByID(id); p != nil {
				items = append(items, *p)
			}
		}
		render(c, 200, cartPage, pageData{Title: "Your cart", Items: items})
	})
	shop.POST("/checkout", func(c *gin.Context) {
		s := session(c)
		if len(s.Cart) == 0 {
			c.Redirect(http.StatusSeeOther, "/shop/cart")
			return
		}
		order := placeOrder(s.Cart)
		sessionsMu.Lock()
		s.Orders = append(s.Orders, order.ID)
		s.Cart = nil
		sessionsMu.Unlock()
		c.Redirect(http.StatusSeeOther, "/shop/orders/"+order.ID)
	})
	shop.GET("/orders/:id", func(c *gin.Context) {
		mu.RLock()
		o, ok := orders[c.Param("id")]
		mu.RUnlock()
		if !ok {
			render(c, 404, orderPage, pageData{Title: "Order not found"})
			return
		}
		render(c, 200, orderPage, pageData{Title: "Order placed", Order: &o})
	})

	login := func(email bool) (gin.HandlerFunc, gin.HandlerFunc) {
		action := "/shop/login"
		if email {
			action = "/shop/login-email"
		}
		get := func(c *gin.Context) {
			render(c, 200, loginPage, pageData{Title: "Log in", Action: action, EmailField: email})
		}
		post := func(c *gin.Context) {
			name, want := c.PostForm("username"), "demo"
			if email {
				name, want = c.PostForm("email"), "demo@example.com"
			}
			if !strings.EqualFold(strings.TrimSpace(name), want) || c.PostForm("password") != "demo-pass" {
				render(c, http.StatusUnauthorized, loginPage, pageData{Title: "Log in", Action: action, EmailField: email, Error: "Wrong username or password."})
				return
			}
			s := session(c)
			sessionsMu.Lock()
			s.User = "demo"
			sessionsMu.Unlock()
			c.Redirect(http.StatusSeeOther, "/shop/account")
		}
		return get, post
	}
	getLogin, postLogin := login(false)
	shop.GET("/login", getLogin)
	shop.POST("/login", postLogin)
	getEmailLogin, postEmailLogin := login(true)
	shop.GET("/login-email", getEmailLogin)
	shop.POST("/login-email", postEmailLogin)

	shop.GET("/logout", func(c *gin.Context) {
		s := session(c)
		sessionsMu.Lock()
		s.User = ""
		sessionsMu.Unlock()
		c.Redirect(http.StatusFound, "/shop")
	})
	shop.GET("/account", func(c *gin.Context) {
		if requireLogin(c) {
			render(c, 200, accountPage, pageData{Title: "My account"})
		}
	})
	shop.GET("/account/orders", func(c *gin.Context) {
		if requireLogin(c) {
			render(c, 200, accountOrdersPage, pageData{Title: "My orders", Orders: session(c).Orders})
		}
	})
	shop.GET("/account/settings", func(c *gin.Context) {
		if requireLogin(c) {
			render(c, 200, settingsPage, pageData{Title: "Settings"})
		}
	})
	shop.POST("/account/settings", func(c *gin.Context) {
		if requireLogin(c) {
			c.Redirect(http.StatusSeeOther, "/shop/account")
		}
	})
	shop.GET("/account/delete", func(c *gin.Context) {
		s := session(c)
		sessionsMu.Lock()
		s.User, s.Cart, s.Orders = "", nil, nil
		sessionsMu.Unlock()
		render(c, 200, deletePage, pageData{Title: "Account deleted"})
	})

	shop.GET("/about", func(c *gin.Context) {
		render(c, 200, aboutPage, pageData{Title: "About"})
	})
	shop.GET("/app", func(c *gin.Context) {
		render(c, 200, appPage, pageData{Title: "Today's deals"})
	})
	shop.GET("/deals/:id", func(c *gin.Context) {
		p := productByID(c.Param("id"))
		if p == nil {
			render(c, 404, dealPage, pageData{Title: "Deal not found"})
			return
		}
		render(c, 200, dealPage, pageData{Title: "Deal", Product: p, DealPrice: p.Price * 0.8})
	})

	staff := shop.Group("/staff", gin.BasicAuth(gin.Accounts{"demo": "demo-pass"}))
	staff.GET("", func(c *gin.Context) {
		render(c, 200, staffPage, pageData{Title: "Staff area"})
	})
	staff.GET("/reports", func(c *gin.Context) {
		mu.RLock()
		n := len(orders)
		mu.RUnlock()
		render(c, 200, reportsPage, pageData{Title: "Sales reports", OrderCount: n})
	})
}

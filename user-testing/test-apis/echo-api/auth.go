package main

// Test-only authentication endpoints so every playground auth method can be
// tried against a local server. The OAuth "provider" auto-approves anyone;
// never use these for anything real.

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	demoClientID     = "opensynapse-demo"
	demoClientSecret = "demo-secret"
	demoUser         = "demo"
	demoPassword     = "demo-pass"
	digestRealm      = "echo-api"
)

type pendingCode struct {
	redirectURI string
	challenge   string
	created     time.Time
}

var (
	oauthMu    sync.Mutex
	codes      = map[string]pendingCode{}
	tokens     = map[string]time.Time{}
	digestNonc = map[string]bool{}
)

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func issueToken() string {
	t := "demo-" + randomHex(16)
	oauthMu.Lock()
	tokens[t] = time.Now().Add(time.Hour)
	oauthMu.Unlock()
	return t
}

func registerAuthRoutes(router *gin.Engine) {
	// OAuth 2.0 authorization endpoint: approves immediately and redirects back.
	router.GET("/oauth/authorize", func(c *gin.Context) {
		if c.Query("client_id") != demoClientID || c.Query("response_type") != "code" {
			c.String(400, "unknown client_id or unsupported response_type")
			return
		}
		redirect, err := url.Parse(c.Query("redirect_uri"))
		if err != nil || redirect.Host == "" {
			c.String(400, "redirect_uri is required")
			return
		}
		code := randomHex(12)
		oauthMu.Lock()
		codes[code] = pendingCode{redirectURI: c.Query("redirect_uri"), challenge: c.Query("code_challenge"), created: time.Now()}
		oauthMu.Unlock()
		q := redirect.Query()
		q.Set("code", code)
		q.Set("state", c.Query("state"))
		redirect.RawQuery = q.Encode()
		c.Redirect(302, redirect.String())
	})

	// OAuth 2.0 token endpoint: authorization_code (with PKCE) and client_credentials.
	router.POST("/oauth/token", func(c *gin.Context) {
		clientID, clientSecret := c.PostForm("client_id"), c.PostForm("client_secret")
		if u, p, ok := c.Request.BasicAuth(); ok {
			clientID, clientSecret = u, p
		}
		if clientID != demoClientID {
			c.JSON(401, gin.H{"error": "invalid_client"})
			return
		}
		switch c.PostForm("grant_type") {
		case "client_credentials":
			if clientSecret != demoClientSecret {
				c.JSON(401, gin.H{"error": "invalid_client"})
				return
			}
		case "authorization_code":
			oauthMu.Lock()
			pc, ok := codes[c.PostForm("code")]
			delete(codes, c.PostForm("code"))
			oauthMu.Unlock()
			if !ok || time.Since(pc.created) > 5*time.Minute || pc.redirectURI != c.PostForm("redirect_uri") {
				c.JSON(400, gin.H{"error": "invalid_grant"})
				return
			}
			if pc.challenge != "" {
				sum := sha256.Sum256([]byte(c.PostForm("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(sum[:]) != pc.challenge {
					c.JSON(400, gin.H{"error": "invalid_grant", "error_description": "PKCE verification failed"})
					return
				}
			} else if clientSecret != demoClientSecret {
				c.JSON(401, gin.H{"error": "invalid_client"})
				return
			}
		default:
			c.JSON(400, gin.H{"error": "unsupported_grant_type"})
			return
		}
		c.JSON(200, gin.H{"access_token": issueToken(), "token_type": "Bearer", "expires_in": 3600, "scope": c.PostForm("scope")})
	})

	// A resource that needs a token from the OAuth endpoints above.
	router.GET("/oauth/me", func(c *gin.Context) {
		t := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		oauthMu.Lock()
		exp, ok := tokens[t]
		oauthMu.Unlock()
		if !ok || time.Now().After(exp) {
			c.JSON(401, gin.H{"error": "invalid_token"})
			return
		}
		c.JSON(200, gin.H{"user": demoUser, "authenticated_with": "oauth2"})
	})

	// HTTP Basic auth.
	router.GET("/auth/basic", func(c *gin.Context) {
		if u, p, ok := c.Request.BasicAuth(); !ok || u != demoUser || p != demoPassword {
			c.Header("WWW-Authenticate", `Basic realm="echo-api"`)
			c.JSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(200, gin.H{"user": demoUser, "authenticated_with": "basic"})
	})

	// API key in a header (X-API-Key) or query parameter (api_key).
	router.GET("/auth/api-key", func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != "demo-key" && c.Query("api_key") != "demo-key" {
			c.JSON(401, gin.H{"error": "missing or wrong API key (use demo-key)"})
			return
		}
		c.JSON(200, gin.H{"authenticated_with": "api_key"})
	})

	// HTTP Digest auth (MD5, qop=auth).
	router.GET("/auth/digest", func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Digest ") || !validDigest(h, c.Request.Method) {
			nonce := randomHex(16)
			oauthMu.Lock()
			digestNonc[nonce] = true
			oauthMu.Unlock()
			c.Header("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", qop="auth", nonce="%s", opaque="%s", algorithm=MD5`, digestRealm, nonce, randomHex(8)))
			c.JSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(200, gin.H{"user": demoUser, "authenticated_with": "digest"})
	})
}

var digestKV = regexp.MustCompile(`(\w+)=("([^"]*)"|[^,\s]+)`)

func validDigest(header, method string) bool {
	p := map[string]string{}
	for _, m := range digestKV.FindAllStringSubmatch(strings.TrimPrefix(header, "Digest "), -1) {
		v := m[2]
		if strings.HasPrefix(v, `"`) {
			v = m[3]
		}
		p[m[1]] = v
	}
	oauthMu.Lock()
	known := digestNonc[p["nonce"]]
	oauthMu.Unlock()
	if !known || p["username"] != demoUser {
		return false
	}
	md := func(s string) string { s2 := md5.Sum([]byte(s)); return hex.EncodeToString(s2[:]) }
	ha1 := md(demoUser + ":" + digestRealm + ":" + demoPassword)
	ha2 := md(method + ":" + p["uri"])
	want := md(ha1 + ":" + p["nonce"] + ":" + p["nc"] + ":" + p["cnonce"] + ":" + p["qop"] + ":" + ha2)
	return p["response"] == want
}

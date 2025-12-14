package backend

import (
	"testing"
	"time"
)

// TestAuthCacheBasicOperations tests the basic get/set operations of the auth cache
func TestAuthCacheBasicOperations(t *testing.T) {
	cache := &authCache{
		cache: make(map[string]*authCacheEntry),
	}

	email := "test@example.com"
	token := "test-token-123"
	expiry := time.Now().Unix() + 3600 // 1 hour from now

	// Test cache miss
	_, _, found := cache.get(email)
	if found {
		t.Error("Expected cache miss for new email, but got a hit")
	}

	// Test cache set
	cache.set(email, token, expiry)

	// Test cache hit
	cachedToken, cachedExpiry, found := cache.get(email)
	if !found {
		t.Error("Expected cache hit after set, but got a miss")
	}
	if cachedToken != token {
		t.Errorf("Expected token %s, got %s", token, cachedToken)
	}
	if cachedExpiry != expiry {
		t.Errorf("Expected expiry %d, got %d", expiry, cachedExpiry)
	}
}

// TestAuthCacheExpiration tests that the BearerToken logic respects expiration
func TestAuthCacheExpiration(t *testing.T) {
	cache := &authCache{
		cache: make(map[string]*authCacheEntry),
	}

	email := "test@example.com"
	token := "test-token-123"

	// Set an expired token
	expiredTime := time.Now().Unix() - 3600 // 1 hour ago
	cache.set(email, token, expiredTime)

	// Get the token
	cachedToken, cachedExpiry, found := cache.get(email)
	if !found {
		t.Error("Expected cache hit, but got a miss")
	}

	// Verify that the expiry is in the past (caller should check this)
	if cachedExpiry > time.Now().Unix() {
		t.Error("Expected expired token, but token is still valid")
	}

	// The cache should still return the token - it's up to BearerToken to check expiry
	if cachedToken != token {
		t.Errorf("Expected token %s, got %s", token, cachedToken)
	}
}

// TestAuthCacheMultipleAccounts tests that the cache can handle multiple accounts
func TestAuthCacheMultipleAccounts(t *testing.T) {
	cache := &authCache{
		cache: make(map[string]*authCacheEntry),
	}

	// Set tokens for multiple accounts
	accounts := map[string]string{
		"user1@example.com": "token1",
		"user2@example.com": "token2",
		"user3@example.com": "token3",
	}

	expiry := time.Now().Unix() + 3600

	for email, token := range accounts {
		cache.set(email, token, expiry)
	}

	// Verify all accounts have their correct tokens
	for email, expectedToken := range accounts {
		cachedToken, _, found := cache.get(email)
		if !found {
			t.Errorf("Cache miss for account %s", email)
		}
		if cachedToken != expectedToken {
			t.Errorf("For account %s, expected token %s, got %s", email, expectedToken, cachedToken)
		}
	}
}

// TestAuthCacheConcurrentAccess tests that the cache is thread-safe
func TestAuthCacheConcurrentAccess(t *testing.T) {
	cache := &authCache{
		cache: make(map[string]*authCacheEntry),
	}

	email := "test@example.com"
	token := "test-token"
	expiry := time.Now().Unix() + 3600

	// Perform concurrent writes and reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		// Concurrent writes
		go func(i int) {
			cache.set(email, token, expiry)
			done <- true
		}(i)

		// Concurrent reads
		go func(i int) {
			cache.get(email)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	cachedToken, _, found := cache.get(email)
	if !found {
		t.Error("Expected cache hit after concurrent operations")
	}
	if cachedToken != token {
		t.Errorf("Expected token %s, got %s", token, cachedToken)
	}
}

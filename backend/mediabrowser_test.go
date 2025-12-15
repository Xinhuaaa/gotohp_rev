package backend

import (
	"testing"
)

func TestMediaBrowser_AccountSwitch(t *testing.T) {
	// Setup mock config
	originalConfig := AppConfig
	defer func() { AppConfig = originalConfig }()

	// Create dummy credentials
	cred1 := "Email=user1@example.com&Token=token1&androidId=id1&app=app&client_sig=sig&lang=en&service=lh2"
	cred2 := "Email=user2@example.com&Token=token2&androidId=id2&app=app&client_sig=sig&lang=en&service=lh2"

	AppConfig.Credentials = []string{cred1, cred2}

	// Helper to mock NewApi behavior indirectly by setting up the expected state
	// Since NewApi relies on global AppConfig and does real HTTP calls (which we can't easily mock without dependency injection),
	// we will partially rely on the fact that NewApi fails if it can't find credentials.
	// However, NewApi creates a client and we don't want real network calls.
	// For this unit test of logic, we mainly care that getAPI *attempts* to create a new API object when the email changes.

	// Create a MediaBrowser
	mb := &MediaBrowser{}

	// 1. Select Account 1
	AppConfig.Selected = "user1@example.com"

	// We expect the first call to succeed in creating an API object (even if it might fail later on network steps in real life, NewApi itself just sets up the struct)
	api1, err := mb.getAPI()
	if err != nil {
		t.Fatalf("Failed to get API for user1: %v", err)
	}
	if api1.Email != "user1@example.com" {
		t.Errorf("Expected api1 email to be user1@example.com, got %s", api1.Email)
	}

	// 2. Select Account 2
	AppConfig.Selected = "user2@example.com"

	// Call getAPI again
	api2, err := mb.getAPI()
	if err != nil {
		t.Fatalf("Failed to get API for user2: %v", err)
	}

	// 3. Verify that we got a new API instance with the correct email
	if api2.Email != "user2@example.com" {
		t.Errorf("Expected api2 email to be user2@example.com, got %s", api2.Email)
	}

	if api1 == api2 {
		t.Error("Expected api1 and api2 to be different instances")
	}

	// 4. Switch back to Account 1
	AppConfig.Selected = "user1@example.com"
	api3, err := mb.getAPI()
	if err != nil {
		t.Fatalf("Failed to revert to API for user1: %v", err)
	}

	if api3.Email != "user1@example.com" {
		t.Errorf("Expected api3 email to be user1@example.com, got %s", api3.Email)
	}

	if api3 == api2 {
		t.Error("Expected api3 and api2 to be different instances")
	}
}

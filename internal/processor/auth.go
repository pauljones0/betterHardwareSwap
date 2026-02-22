package processor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// HandleRedditLogin redirects the user to Reddit's OAuth consent screen.
// Expects `?user_id=12345` query param from the Discord dashboard.
func HandleRedditLogin(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("BACKEND_API_REDDIT_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("REDDIT_CLIENT_ID") // Fallback for backwards compat
	}
	redirectURI := os.Getenv("BACKEND_API_REDDIT_REDIRECT_URI")

	if clientID == "" || redirectURI == "" {
		http.Error(w, "server misconfiguration: missing oauth vars", http.StatusInternalServerError)
		return
	}

	// Generate a random state. In a real app we'd save this to a cookie/session to prevent CSRF,
	// but for this utility we map the state to the user_id so we know who they are on callback.
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	stateStr := hex.EncodeToString(stateBytes)

	// Encode user_id into state (format: {random_hex}:{user_id})
	state := fmt.Sprintf("%s:%s", stateStr, userID)

	authURL := fmt.Sprintf(
		"https://www.reddit.com/api/v1/authorize?client_id=%s&response_type=code&state=%s&redirect_uri=%s&duration=permanent&scope=read",
		clientID,
		url.QueryEscape(state),
		url.QueryEscape(redirectURI),
	)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleRedditCallback handles the redirect from Reddit with the auth code.
func HandleRedditCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errQuery := r.URL.Query().Get("error")

	if errQuery != "" {
		http.Error(w, fmt.Sprintf("Reddit returned an error: %s", errQuery), http.StatusBadRequest)
		return
	}

	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	// Extract userID from state
	// State format: {random_hex}:{user_id}
	var userID string
	for i := len(state) - 1; i >= 0; i-- {
		if state[i] == ':' {
			userID = state[i+1:]
			break
		}
	}

	if userID == "" {
		http.Error(w, "invalid state format (could not extract user id)", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("BACKEND_API_REDDIT_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("REDDIT_CLIENT_ID")
	}
	clientSecret := os.Getenv("BACKEND_API_REDDIT_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("REDDIT_CLIENT_SECRET")
	}
	redirectURI := os.Getenv("BACKEND_API_REDDIT_REDIRECT_URI")

	// 1. Exchange the code for tokens
	tokenResp, err := reddit.ExchangeCodeForToken(code, redirectURI, clientID, clientSecret)
	if err != nil {
		log.Printf("Token exchange error: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	// 2. Encrypt the tokens
	encAccess, err := reddit.Encrypt(tokenResp.AccessToken)
	if err != nil {
		log.Printf("Encryption error: %v", err)
		http.Error(w, "Internal encryption error", http.StatusInternalServerError)
		return
	}

	encRefresh, err := reddit.Encrypt(tokenResp.RefreshToken)
	if err != nil {
		log.Printf("Encryption error (refresh): %v", err)
		http.Error(w, "Internal encryption error", http.StatusInternalServerError)
		return
	}

	// 3. Save to Firestore
	ctx := context.Background()
	projectID := os.Getenv("GCP_PROJECT_ID")
	db, err := store.NewStore(ctx, projectID)
	if err != nil {
		log.Printf("Firestore init error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	cred := store.UserCredential{
		UserID:       userID,
		AccessToken:  encAccess,
		RefreshToken: encRefresh,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if err := db.SaveUserCredential(ctx, cred); err != nil {
		log.Printf("Firestore save error: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Success! Reddit account successfully linked to Discord User %s. You may now close this tab.", userID)
}

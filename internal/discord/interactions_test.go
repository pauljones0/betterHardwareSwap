package discord

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHandleInteraction_Ping(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	os.Setenv("DISCORD_PUBLIC_KEY", pubHex)
	defer os.Unsetenv("DISCORD_PUBLIC_KEY")

	interaction := discordgo.Interaction{
		Type: discordgo.InteractionPing,
	}
	body, _ := json.Marshal(interaction)

	timestamp := "123456789"
	msg := append([]byte(timestamp), body...)
	sig := ed25519.Sign(priv, msg)

	req := httptest.NewRequest("POST", "/interactions", bytes.NewReader(body))
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	req.Header.Set("X-Signature-Timestamp", timestamp)

	rr := httptest.NewRecorder()
	HandleInteraction(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp discordgo.InteractionResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Type != discordgo.InteractionResponsePong {
		t.Errorf("expected response type pong, got %v", resp.Type)
	}
}

func TestHandleInteraction_Unauthorized(t *testing.T) {
	os.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(make([]byte, 32)))
	defer os.Unsetenv("DISCORD_PUBLIC_KEY")

	req := httptest.NewRequest("POST", "/interactions", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Signature-Ed25519", "invalid")
	req.Header.Set("X-Signature-Timestamp", "invalid")

	rr := httptest.NewRecorder()
	HandleInteraction(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

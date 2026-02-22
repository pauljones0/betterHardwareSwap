package store

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Store represents a connection to the Firestore database.
type Store struct {
	client *firestore.Client
}

// ServerConfig stores Discord server configuration.
type ServerConfig struct {
	FeedChannelID string    `firestore:"feed_channel_id"`
	PingChannelID string    `firestore:"ping_channel_id"`
	UpdatedAt     time.Time `firestore:"updated_at"`
}

// AlertRule represents a single user's keyword alert.
type AlertRule struct {
	ID        string    `firestore:"-"`
	UserID    string    `firestore:"user_id"`
	ServerID  string    `firestore:"server_id"`
	MustHave  []string  `firestore:"must_have"` // AND
	AnyOf     []string  `firestore:"any_of"`    // OR
	MustNot   []string  `firestore:"must_not"`  // NOT
	RawQuery  string    `firestore:"raw_query"` // What the user originally typed
	CreatedAt time.Time `firestore:"created_at"`
}

// PostRecord maps a Reddit post ID to a Discord message ID to allow updating/striking-through.
type PostRecord struct {
	RedditID     string    `firestore:"reddit_id"`
	DiscordMsgID string    `firestore:"discord_msg_id"`
	PostedAt     time.Time `firestore:"posted_at"`
}

// AnalyticsRecord stores information about how an alert was created to evaluate AI effectiveness.
type AnalyticsRecord struct {
	ID                 string    `firestore:"-"`
	FlowType           string    `firestore:"flow_type"` // "wizard" or "manual"
	OriginalUserPrompt string    `firestore:"original_user_prompt,omitempty"`
	AISuggestedQuery   string    `firestore:"ai_suggested_query,omitempty"`
	FinalSavedQuery    string    `firestore:"final_saved_query,omitempty"`
	Outcome            string    `firestore:"outcome"` // e.g., Accepted_As_Is, Edited, Cancelled, Manual_Entry_Success
	EditCount          int       `firestore:"edit_count"`
	CreatedAt          time.Time `firestore:"created_at"`
}

// SystemPrompt stores the dynamically updated system instructions for the AI model.
type SystemPrompt struct {
	PromptText string    `firestore:"prompt_text"`
	UpdatedAt  time.Time `firestore:"updated_at"`
}

// NewStore initializes a new Firestore client using application default credentials.
func NewStore(ctx context.Context, projectID string) (*Store, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %v", err)
	}
	return &Store{client: client}, nil
}

// Close closes the Firestore client.
func (s *Store) Close() error {
	return s.client.Close()
}

// --- Server Configs ---

// SaveServerConfig saves or updates the feed and ping channels for a given Discord server.
func (s *Store) SaveServerConfig(ctx context.Context, serverID string, cfg ServerConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := s.client.Collection("servers").Doc(serverID).Set(ctx, cfg)
	return err
}

// GetServerConfig retrieves the server config for a given Discord server ID.
func (s *Store) GetServerConfig(ctx context.Context, serverID string) (*ServerConfig, error) {
	doc, err := s.client.Collection("servers").Doc(serverID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var cfg ServerConfig
	if err := doc.DataTo(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// --- Alerts ---

// AddAlert adds a new alert rule for a user on a specific server.
func (s *Store) AddAlert(ctx context.Context, rule AlertRule) error {
	rule.CreatedAt = time.Now()
	_, _, err := s.client.Collection("alerts").Add(ctx, rule)
	return err
}

// GetUserAlerts retrieves all alerts for a specific user on a specific server.
func (s *Store) GetUserAlerts(ctx context.Context, serverID, userID string) ([]AlertRule, error) {
	var alerts []AlertRule
	iter := s.client.Collection("alerts").
		Where("server_id", "==", serverID).
		Where("user_id", "==", userID).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var alert AlertRule
		if err := doc.DataTo(&alert); err != nil {
			return nil, err
		}
		alert.ID = doc.Ref.ID
		alerts = append(alerts, alert)
	}

	// Sort alerts descending by creation time in memory to avoid needing a Firestore composite index
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].CreatedAt.After(alerts[j].CreatedAt)
	})

	return alerts, nil
}

// DeleteAlert removes an alert rule by its Firestore document ID (not the Discord interaction ID).
func (s *Store) DeleteAlert(ctx context.Context, docID string) error {
	_, err := s.client.Collection("alerts").Doc(docID).Delete(ctx)
	return err
}

// DeleteAllUserAlerts removes every alert a specific user has registered on a given server.
func (s *Store) DeleteAllUserAlerts(ctx context.Context, serverID, userID string) error {
	alerts, err := s.GetUserAlerts(ctx, serverID, userID)
	if err != nil {
		return err
	}

	batch := s.client.Batch()
	for _, alert := range alerts {
		ref := s.client.Collection("alerts").Doc(alert.ID)
		batch.Delete(ref)
	}

	if len(alerts) > 0 {
		_, err = batch.Commit(ctx)
		return err
	}
	return nil
}

// GetAllAlerts retrieves all alerts across all servers. Used heavily by the scraper deduplication logic.
func (s *Store) GetAllAlerts(ctx context.Context) ([]AlertRule, error) {
	var alerts []AlertRule
	iter := s.client.Collection("alerts").Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var alert AlertRule
		if err := doc.DataTo(&alert); err != nil {
			return nil, err
		}
		alert.ID = doc.Ref.ID
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// --- Posts ---

// SavePostRecord stores the mapping between a Reddit Post ID and the Discord Message ID it generated.
func (s *Store) SavePostRecord(ctx context.Context, redditID, discordMsgID string) error {
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ref := s.client.Collection("posts").Doc(redditID)
		return tx.Set(ref, PostRecord{
			RedditID:     redditID,
			DiscordMsgID: discordMsgID,
			PostedAt:     time.Now(),
		})
	})
}

// GetPostRecord retrieves a post record to find the matching Discord Message ID.
func (s *Store) GetPostRecord(ctx context.Context, redditID string) (*PostRecord, error) {
	doc, err := s.client.Collection("posts").Doc(redditID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var pr PostRecord
	if err := doc.DataTo(&pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// TrimOldPosts hard-deletes posts older than the 500 most recent ones to keep the database exceptionally lean.
func (s *Store) TrimOldPosts(ctx context.Context) error {
	// 1. Get all post documents, ordered by creation time descending.
	iter := s.client.Collection("posts").
		OrderBy("posted_at", firestore.Desc).
		Documents(ctx)

	count := 0
	batch := s.client.Batch()
	docsToDelete := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// If we fail here, we just log and return.
			// Trimming isn't critical, it'll just try again next time.
			log.Printf("Error iterating posts during trim: %v", err)
			return err
		}

		count++
		// If we've seen more than 500, queue this document for deletion.
		if count > 500 {
			batch.Delete(doc.Ref)
			docsToDelete++

			// Firestore batches are limited to 500 operations.
			// If we hit it, commit and start a new batch.
			if docsToDelete == 500 {
				if _, err := batch.Commit(ctx); err != nil {
					log.Printf("Error committing chunked batch delete during trim: %v", err)
					return err
				}
				batch = s.client.Batch()
				docsToDelete = 0
			}
		}
	}

	// Commit any remaining deletions.
	if docsToDelete > 0 {
		if _, err := batch.Commit(ctx); err != nil {
			log.Printf("Error committing final batch delete during trim: %v", err)
			return err
		}
		log.Printf("Trimmed %d old posts from Firestore.", docsToDelete)
	}

	return nil
}

// --- Analytics ---

// SaveAnalytics saves an interaction record for AI query generation analytics.
func (s *Store) SaveAnalytics(ctx context.Context, record AnalyticsRecord) error {
	record.CreatedAt = time.Now()
	_, _, err := s.client.Collection("ai_query_analytics").Add(ctx, record)
	return err
}

// GetUnprocessedAnalyticsByFlow grabs up to `limit` records from the analytics collection for a specific AI module.
func (s *Store) GetUnprocessedAnalyticsByFlow(ctx context.Context, flowType string, limit int) ([]AnalyticsRecord, error) {
	var records []AnalyticsRecord
	iter := s.client.Collection("ai_query_analytics").
		Where("flow_type", "==", flowType).
		OrderBy("created_at", firestore.Asc).
		Limit(limit).
		Documents(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var rec AnalyticsRecord
		if err := doc.DataTo(&rec); err != nil {
			continue // skip malformed
		}
		rec.ID = doc.Ref.ID
		records = append(records, rec)
	}

	return records, nil
}

// DeleteAnalyticsChunk deletes a specific set of analytics records by their document IDs.
func (s *Store) DeleteAnalyticsChunk(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	batch := s.client.Batch()
	for _, id := range ids {
		ref := s.client.Collection("ai_query_analytics").Doc(id)
		batch.Delete(ref)
	}
	_, err := batch.Commit(ctx)
	return err
}

// --- Dynamic AI Prompts ---

// GetSystemPrompt retrieves the stored System Prompt (e.g. for "wizard" or "manual").
func (s *Store) GetSystemPrompt(ctx context.Context, key string) (string, error) {
	doc, err := s.client.Collection("system_prompts").Doc(key).Get(ctx)
	if err != nil {
		return "", err
	}
	var sp SystemPrompt
	if err := doc.DataTo(&sp); err != nil {
		return "", err
	}
	return sp.PromptText, nil
}

// SetSystemPrompt saves a new System Prompt definition.
func (s *Store) SetSystemPrompt(ctx context.Context, key, promptText string) error {
	sp := SystemPrompt{
		PromptText: promptText,
		UpdatedAt:  time.Now(),
	}
	_, err := s.client.Collection("system_prompts").Doc(key).Set(ctx, sp)
	return err
}

package message

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

func openMessageTestStore(t *testing.T) (*Store, *database.DB) {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return NewStore(db), db
}

func seedConversationListTestData(t *testing.T, store *Store, db *database.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO accounts (id, name, email, imap_host, smtp_host, username, enabled)
		VALUES ('acc-1', 'Test', 'test@example.com', 'imap.example.com', 'smtp.example.com', 'test@example.com', 1)
	`)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO folders (id, account_id, name, path, folder_type)
		VALUES ('folder-inbox', 'acc-1', 'Inbox', 'INBOX', 'inbox')
	`)
	if err != nil {
		t.Fatalf("insert folder: %v", err)
	}

	base := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	messages := []*Message{
		{
			ID:          "msg-old",
			AccountID:   "acc-1",
			FolderID:    "folder-inbox",
			UID:         1,
			MessageID:   "<old@example.com>",
			ThreadID:    "thread-1",
			Subject:     "Thread subject",
			FromName:    "Alice",
			FromEmail:   "alice@example.com",
			Date:        base,
			Snippet:     "zzz older snippet",
			BodyFetched: true,
		},
		{
			ID:          "msg-new",
			AccountID:   "acc-1",
			FolderID:    "folder-inbox",
			UID:         2,
			MessageID:   "<new@example.com>",
			ThreadID:    "thread-1",
			Subject:     "Thread subject",
			FromName:    "Bob",
			FromEmail:   "bob@example.com",
			Date:        base.Add(time.Hour),
			Snippet:     "aaa latest snippet",
			BodyFetched: true,
		},
	}

	for _, msg := range messages {
		if err := store.Create(msg); err != nil {
			t.Fatalf("Create(%s) error = %v", msg.ID, err)
		}
	}
}

func TestConversationSummariesUseLatestMessageSnippet(t *testing.T) {
	store, db := openMessageTestStore(t)
	seedConversationListTestData(t, store, db)

	folderConversations, err := store.ListConversationsByFolder("folder-inbox", 0, 10, "newest", "")
	if err != nil {
		t.Fatalf("ListConversationsByFolder() error = %v", err)
	}
	if len(folderConversations) != 1 {
		t.Fatalf("ListConversationsByFolder() returned %d conversations, want 1", len(folderConversations))
	}
	if got, want := folderConversations[0].Snippet, "aaa latest snippet"; got != want {
		t.Fatalf("folder conversation snippet = %q, want %q", got, want)
	}

	unifiedConversations, err := store.ListConversationsUnifiedInbox(0, 10, "newest", "")
	if err != nil {
		t.Fatalf("ListConversationsUnifiedInbox() error = %v", err)
	}
	if len(unifiedConversations) != 1 {
		t.Fatalf("ListConversationsUnifiedInbox() returned %d conversations, want 1", len(unifiedConversations))
	}
	if got, want := unifiedConversations[0].Snippet, "aaa latest snippet"; got != want {
		t.Fatalf("unified conversation snippet = %q, want %q", got, want)
	}

	conversation, err := store.GetConversation("thread-1", "folder-inbox")
	if err != nil {
		t.Fatalf("GetConversation() error = %v", err)
	}
	if conversation == nil {
		t.Fatal("GetConversation() returned nil")
	}
	if got, want := conversation.Snippet, "aaa latest snippet"; got != want {
		t.Fatalf("full conversation snippet = %q, want %q", got, want)
	}
}

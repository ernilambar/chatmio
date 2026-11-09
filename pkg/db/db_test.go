package db

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "chats.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return store
}

func TestDeleteMessagesAfterRemovesFollowingMessagesAndAttachments(t *testing.T) {
	store := openTestStore(t)

	thread := Thread{ID: "t1", Title: "New chat", CreatedAt: 1, UpdatedAt: 1}
	if err := store.CreateThread(thread); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	messages := []Message{
		{ID: "m1", ThreadID: "t1", Role: "user", Content: "first", CreatedAt: 2},
		{ID: "m2", ThreadID: "t1", Role: "assistant", Content: "reply", CreatedAt: 3},
		{ID: "m3", ThreadID: "t1", Role: "user", Content: "second", CreatedAt: 4, Attachments: []Attachment{
			{ID: "a3", Name: "note.txt", MimeType: "text/plain", Data: []byte("note"), CreatedAt: 4},
		}},
		{ID: "m4", ThreadID: "t1", Role: "assistant", Content: "reply two", CreatedAt: 5, Attachments: []Attachment{
			{ID: "a4", Name: "image.png", MimeType: "image/png", Data: []byte("png"), CreatedAt: 5},
		}},
	}
	for _, message := range messages {
		if err := store.AddMessage(message); err != nil {
			t.Fatalf("AddMessage(%s): %v", message.ID, err)
		}
	}

	if err := store.DeleteMessagesAfter("t1", "m2"); err != nil {
		t.Fatalf("DeleteMessagesAfter: %v", err)
	}

	remaining, err := store.ListMessages("t1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(remaining) != 2 || remaining[0].ID != "m1" || remaining[1].ID != "m2" {
		t.Fatalf("remaining messages = %+v, want m1, m2", remaining)
	}

	var attachments int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM attachments`).Scan(&attachments); err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if attachments != 0 {
		t.Fatalf("attachments after truncation = %d, want 0", attachments)
	}
}

func TestUpdateMessageContent(t *testing.T) {
	store := openTestStore(t)

	if err := store.CreateThread(Thread{ID: "t1", Title: "Chat", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if err := store.AddMessage(Message{
		ID: "m1", ThreadID: "t1", Role: "user", Content: "before", CreatedAt: 2,
	}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if err := store.UpdateMessageContent("m1", "after"); err != nil {
		t.Fatalf("UpdateMessageContent: %v", err)
	}

	messages, err := store.ListMessages("t1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "after" {
		t.Fatalf("messages = %+v, want one message with content %q", messages, "after")
	}
}

func TestSetThreadTitleIfUnchangedKeepsUserRename(t *testing.T) {
	store := openTestStore(t)

	if err := store.CreateThread(Thread{ID: "t1", Title: "New chat", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	changed, err := store.SetThreadTitleIfUnchanged("t1", "New chat", "Generated title")
	if err != nil {
		t.Fatalf("SetThreadTitleIfUnchanged: %v", err)
	}
	if !changed {
		t.Fatal("first title update reported no change")
	}

	// A rename that lands while the title request is in flight must win.
	if err := store.UpdateThreadMeta("t1", "User title", 2); err != nil {
		t.Fatalf("UpdateThreadMeta: %v", err)
	}
	changed, err = store.SetThreadTitleIfUnchanged("t1", "New chat", "Stale title")
	if err != nil {
		t.Fatalf("SetThreadTitleIfUnchanged: %v", err)
	}
	if changed {
		t.Fatal("stale title overwrote the user rename")
	}

	thread, err := store.GetThread("t1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if thread.Title != "User title" {
		t.Fatalf("title = %q, want %q", thread.Title, "User title")
	}
}

func TestTouchThreadKeepsTitle(t *testing.T) {
	store := openTestStore(t)

	if err := store.CreateThread(Thread{ID: "t1", Title: "Keep me", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if err := store.TouchThread("t1", 99); err != nil {
		t.Fatalf("TouchThread: %v", err)
	}

	thread, err := store.GetThread("t1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if thread.Title != "Keep me" || thread.UpdatedAt != 99 {
		t.Fatalf("thread = %+v, want title %q and updatedAt 99", thread, "Keep me")
	}
}

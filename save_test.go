package main

import (
	"os"
	"path/filepath"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/cache"
	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestSaveConversation(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "continued"
		}
		for _, failDB := range []bool{false, true} {
			result := "success"
			if failDB {
				result = "database failure"
			}
			t.Run(name+"/"+result, func(t *testing.T) {
				originalConfig, originalDB := config, db
				t.Cleanup(func() { config, db = originalConfig, originalDB })
				db = testDB(t)
				dir := t.TempDir()
				id := newConversationID()
				config = Config{
					CachePath: dir, API: "local", Model: "test", Quiet: true,
					cacheWriteToID: id, cacheWriteToTitle: "new title",
				}
				conversations, err := cache.NewConversations(dir)
				require.NoError(t, err)
				bodyPath := filepath.Join(dir, "conversations", id+".gob")
				oldMessages := []proto.Message{{Role: proto.RoleUser, Content: "original question"}}
				var oldBody []byte
				var oldIndex *Conversation
				if existing {
					require.NoError(t, conversations.Write(id, &oldMessages))
					require.NoError(t, db.Save(id, "old title", "old-api", "old-model"))
					oldBody, err = os.ReadFile(bodyPath)
					require.NoError(t, err)
					oldIndex, err = db.Find(id)
					require.NoError(t, err)
				}
				if failDB {
					_, err := db.db.Exec("PRAGMA query_only=ON")
					require.NoError(t, err)
				}
				messages := append([]proto.Message{}, oldMessages...)
				messages = append(messages, proto.Message{Role: proto.RoleAssistant, Content: "new answer"})
				err = saveConversation(&Mods{messages: messages})
				if failDB {
					require.Error(t, err)
					if existing {
						body, err := os.ReadFile(bodyPath)
						require.NoError(t, err)
						require.Equal(t, oldBody, body)
						index, err := db.Find(id)
						require.NoError(t, err)
						require.Equal(t, oldIndex, index)
						var restored []proto.Message
						require.NoError(t, conversations.Read(id, &restored))
						require.Equal(t, oldMessages, restored)
					} else {
						_, err := os.Stat(bodyPath)
						require.ErrorIs(t, err, os.ErrNotExist)
						_, err = db.Find(id)
						require.ErrorIs(t, err, errNoMatches)
					}
				} else {
					require.NoError(t, err)
					var saved []proto.Message
					require.NoError(t, conversations.Read(id, &saved))
					require.Equal(t, messages, saved)
					index, err := db.Find(id)
					require.NoError(t, err)
					require.Equal(t, "new title", index.Title)
					require.Equal(t, "local", *index.API)
					require.Equal(t, "test", *index.Model)
				}
				entries, err := os.ReadDir(filepath.Dir(bodyPath))
				require.NoError(t, err)
				if failDB && !existing {
					require.Empty(t, entries)
				} else {
					require.Len(t, entries, 1)
					require.Equal(t, id+".gob", entries[0].Name())
				}
			})
		}
	}
}

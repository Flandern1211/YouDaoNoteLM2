package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/agentcontext"
)

func TestDisabledMemoryProvider_ReturnsNil(t *testing.T) {
	provider := NewDisabledMemoryProvider()

	candidates, err := provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID:         1,
		Query:          "test query",
		CandidateLimit: 10,
	})

	require.NoError(t, err)
	assert.Nil(t, candidates, "DisabledMemoryProvider 应返回 nil 候选")
}

func TestDelegatingMemoryProvider_WithResults(t *testing.T) {
	searcher := &fakeMemorySearcher{
		results: []MemoryResult{
			{ID: "mem-1", Content: "memory 1", Score: 0.9, Importance: 0.8, Pinned: true},
			{ID: "mem-2", Content: "memory 2", Score: 0.7, Importance: 0.5},
		},
	}

	provider := NewDelegatingMemoryProvider(searcher)

	candidates, err := provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID:         1,
		Query:          "test query",
		CandidateLimit: 10,
	})

	require.NoError(t, err)
	require.Len(t, candidates, 2)

	assert.Equal(t, "mem-1", candidates[0].ID)
	assert.Equal(t, "memory 1", candidates[0].Content)
	assert.Equal(t, 0.9, candidates[0].Score)
	assert.True(t, candidates[0].Pinned)
	assert.Equal(t, agentcontext.SensitivityLow, candidates[0].Sensitivity)
	assert.Equal(t, "delegating_memory", candidates[0].Provenance.Provider)

	assert.Equal(t, "mem-2", candidates[1].ID)
	assert.False(t, candidates[1].Pinned)
}

func TestDelegatingMemoryProvider_NilSearcher(t *testing.T) {
	provider := NewDelegatingMemoryProvider(nil)

	candidates, err := provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID: 1,
		Query:  "test",
	})

	require.NoError(t, err)
	assert.Nil(t, candidates)
}

func TestDelegatingMemoryProvider_SearchError(t *testing.T) {
	searcher := &fakeMemorySearcher{
		err: errors.New("search failed"),
	}

	provider := NewDelegatingMemoryProvider(searcher)

	_, err := provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID: 1,
		Query:  "test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestDelegatingMemoryProvider_EmptyResults(t *testing.T) {
	searcher := &fakeMemorySearcher{
		results: []MemoryResult{},
	}

	provider := NewDelegatingMemoryProvider(searcher)

	candidates, err := provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID: 1,
		Query:  "no results",
	})

	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestDelegatingMemoryProvider_LimitPassthrough(t *testing.T) {
	searcher := &fakeMemorySearcher{
		results: []MemoryResult{
			{ID: "mem-1", Content: "memory 1"},
		},
	}

	provider := NewDelegatingMemoryProvider(searcher)

	// 验证 limit 透传到 searcher
	_, _ = provider.SearchMemory(context.Background(), agentcontext.MemoryQuery{
		UserID:         42,
		Query:          "query",
		CandidateLimit: 5,
	})

	assert.Equal(t, uint(42), searcher.lastUserID)
	assert.Equal(t, "query", searcher.lastQuery)
	assert.Equal(t, 5, searcher.lastLimit)
}

// --- fakes ---

type fakeMemorySearcher struct {
	results    []MemoryResult
	err        error
	lastUserID uint
	lastQuery  string
	lastLimit  int
}

func (f *fakeMemorySearcher) Search(_ context.Context, userID uint, query string, limit int) ([]MemoryResult, error) {
	f.lastUserID = userID
	f.lastQuery = query
	f.lastLimit = limit
	return f.results, f.err
}

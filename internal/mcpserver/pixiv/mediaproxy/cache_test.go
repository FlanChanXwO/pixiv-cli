package mediaproxy

import (
	"github.com/stretchr/testify/require"
	"testing"
	"testing/synctest"
	"time"
)

func TestContentCacheBudgetLRUAndTTL(t *testing.T) {
	c := newContentCache(6)
	expiry := time.Now().Add(time.Hour)
	c.put("a", []byte("aaa"), expiry)
	c.put("b", []byte("bbb"), expiry)
	data, ok := c.get("a")
	require.True(t, ok)
	require.Equal(t, "aaa", string(data[0]))
	c.put("c", []byte("ccc"), expiry)
	_, ok = c.get("b")
	require.False(t, ok)
	c.put("large", []byte("1234567"), expiry)
	_, ok = c.get("large")
	require.False(t, ok)
	c.put("expired", []byte("x"), time.Now().Add(-time.Second))
	_, ok = c.get("expired")
	require.False(t, ok)
	disabled := newContentCache(0)
	disabled.put("a", []byte("a"), expiry)
	_, ok = disabled.get("a")
	require.False(t, ok)
}

func TestCacheFillingBuffersShareBudget(t *testing.T) {
	c := newContentCache(6)
	filling := c.begin()
	defer filling.abort()
	require.True(t, filling.append([]byte("aaaa")))
	c.put("b", []byte("bbb"), time.Now().Add(time.Hour))
	_, ok := c.get("b")
	require.False(t, ok)
	require.False(t, filling.append([]byte("aaa")), "oversized fill abandons caching without retaining its prefix")
	require.EqualValues(t, 0, c.used)
	c.put("a", []byte("aaaaaa"), time.Now().Add(time.Hour))
	_, release, ok := c.load("a")
	require.True(t, ok)
	c.put("b", []byte("bbb"), time.Now().Add(time.Hour))
	_, ok = c.get("b")
	require.False(t, ok, "active cached transfers remain charged to the budget")
	release()
	c.put("b", []byte("bbb"), time.Now().Add(time.Hour))
	_, ok = c.get("b")
	require.True(t, ok)
}

func TestCacheTTLEndsAtAuthorizationOrSessionExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newContentCache(10)
		cache.put("a", []byte("a"), time.Now().Add(time.Minute))
		time.Sleep(time.Minute)
		_, ok := cache.get("a")
		require.False(t, ok)
	})
	s := &Service{reads: &readState{config: Config{TTL: 15 * time.Minute}}}
	expiry := time.Now().Add(time.Minute)
	require.Equal(t, expiry, s.expiry(claims{Expires: expiry}, Session{}))
	earlier := expiry.Add(-time.Second)
	require.Equal(t, earlier, s.expiry(claims{Expires: expiry}, Session{Expires: earlier}))
}

func TestIdleExpiredCacheReclaimsBudgetWithoutAnotherRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newContentCache(10)
		cache.put("a", []byte("a"), time.Now().Add(time.Minute))
		time.Sleep(time.Minute)
		synctest.Wait()
		require.Zero(t, cache.used)
	})
}

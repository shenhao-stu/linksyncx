package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	fingerprintKeyPrefix   = "fingerprint:"
	fingerprintTTL         = 7 * 24 * time.Hour // 7天，配合每24小时懒续期可保持活跃账号永不过期
	maskedSessionKeyPrefix = "masked_session:"
	maskedSessionTTL       = 15 * time.Minute

	// 账号级会话键（新键按 {platform}:{layer}:{account}:{sub} 命名）。
	// ambient：账号空闲时的环境会话；last：最近活跃的上游会话。
	accountSessionKeyPrefix = "claude:session:"
	accountSessionTTL       = 15 * time.Minute
)

func ambientSessionKey(accountID int64) string {
	return fmt.Sprintf("%s%d:ambient", accountSessionKeyPrefix, accountID)
}

func lastActiveSessionKey(accountID int64) string {
	return fmt.Sprintf("%s%d:last", accountSessionKeyPrefix, accountID)
}

// fingerprintKey generates the Redis key for account fingerprint cache.
func fingerprintKey(accountID int64) string {
	return fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
}

// maskedSessionKey generates the Redis key for masked session ID cache.
func maskedSessionKey(accountID int64) string {
	return fmt.Sprintf("%s%d", maskedSessionKeyPrefix, accountID)
}

type identityCache struct {
	rdb *redis.Client
}

func NewIdentityCache(rdb *redis.Client) service.IdentityCache {
	return &identityCache{rdb: rdb}
}

func (c *identityCache) GetFingerprint(ctx context.Context, accountID int64) (*service.Fingerprint, error) {
	key := fingerprintKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeFingerprint(val)
}

func decodeFingerprint(val string) (*service.Fingerprint, error) {
	var fp service.Fingerprint
	if err := json.Unmarshal([]byte(val), &fp); err != nil {
		return nil, err
	}
	if fp.ClientID == "" {
		return nil, fmt.Errorf("stored account identity has no client ID")
	}
	return &fp, nil
}

var refreshFingerprintScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then
    local ok, identity = pcall(cjson.decode, current)
    if not ok or type(identity) ~= 'table' or type(identity.ClientID) ~= 'string' or identity.ClientID == '' then
        return redis.error_reply('invalid stored account identity')
    end
    if identity.ClientID ~= ARGV[2] then return current end
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
return ARGV[1]
`)

func (c *identityCache) SetFingerprint(ctx context.Context, accountID int64, fp *service.Fingerprint) (*service.Fingerprint, error) {
	if fp == nil || fp.ClientID == "" {
		return nil, fmt.Errorf("account identity requires a client ID")
	}
	val, err := json.Marshal(fp)
	if err != nil {
		return nil, err
	}
	stored, err := refreshFingerprintScript.Run(ctx, c.rdb, []string{fingerprintKey(accountID)}, val, fp.ClientID, fingerprintTTL.Milliseconds()).Text()
	if err != nil {
		return nil, err
	}
	return decodeFingerprint(stored)
}

// Creation and winner selection share one Redis operation. Existing malformed
// records are returned for decoding and never silently replaced with a new identity.
var createFingerprintScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then return current end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return ARGV[1]
`)

func (c *identityCache) CreateFingerprint(ctx context.Context, accountID int64, fp *service.Fingerprint) (*service.Fingerprint, error) {
	if fp == nil || fp.ClientID == "" {
		return nil, fmt.Errorf("account identity requires a client ID")
	}
	val, err := json.Marshal(fp)
	if err != nil {
		return nil, err
	}
	stored, err := createFingerprintScript.Run(ctx, c.rdb, []string{fingerprintKey(accountID)}, val, fingerprintTTL.Milliseconds()).Text()
	if err != nil {
		return nil, err
	}
	return decodeFingerprint(stored)
}

var maskedSessionScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current or current == '' then current = ARGV[1] end
redis.call('SET', KEYS[1], current, 'PX', ARGV[2])
return current
`)

func (c *identityCache) GetOrCreateMaskedSessionID(ctx context.Context, accountID int64, candidate string) (string, error) {
	if candidate == "" {
		return "", fmt.Errorf("session candidate must not be empty")
	}
	return maskedSessionScript.Run(ctx, c.rdb, []string{maskedSessionKey(accountID)}, candidate, maskedSessionTTL.Milliseconds()).Text()
}

// GetOrCreateAmbientSessionID 与伪装会话同一语义（原子 get-or-create + 滑动 TTL），
// 键不同：环境会话只服务账号空闲时没有可映射会话的请求。
func (c *identityCache) GetOrCreateAmbientSessionID(ctx context.Context, accountID int64, candidate string) (string, error) {
	if candidate == "" {
		return "", fmt.Errorf("session candidate must not be empty")
	}
	return maskedSessionScript.Run(ctx, c.rdb, []string{ambientSessionKey(accountID)}, candidate, accountSessionTTL.Milliseconds()).Text()
}

func (c *identityCache) SetLastActiveSessionID(ctx context.Context, accountID int64, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session id must not be empty")
	}
	return c.rdb.Set(ctx, lastActiveSessionKey(accountID), sessionID, accountSessionTTL).Err()
}

func (c *identityCache) GetLastActiveSessionID(ctx context.Context, accountID int64) (string, error) {
	val, err := c.rdb.Get(ctx, lastActiveSessionKey(accountID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

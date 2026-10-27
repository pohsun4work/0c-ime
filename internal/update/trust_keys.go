package update

import (
	"crypto/ed25519"
	"encoding/base64"
)

// hoshino-2026-10：2026-10-28 產生的 fork 發行金鑰，取代官方的 0c-release-2026-09；
// 私鑰只存在 GitHub environment「release」的 secret，fork 的更新器只接受這把簽的索引。
var releaseKeyText = []string{"XSWXhXI6nLEIffHczzOw9N5BZk/h3CvCIFmN/fIoQ1o="}

func releaseKeys() []ed25519.PublicKey { return decodeKeys(releaseKeyText) }

func decodeKeys(texts []string) []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	for _, t := range texts {
		if raw, err := base64.StdEncoding.DecodeString(t); err == nil && len(raw) == ed25519.PublicKeySize {
			keys = append(keys, ed25519.PublicKey(raw))
		}
	}
	return keys
}

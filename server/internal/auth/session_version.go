package auth

import "github.com/golang-jwt/jwt/v5"

// SessionVersionClaim ties a UI session token to the user's current
// auth_version. Password changes and account deletion bump/invalidate that
// version, which is what lets a long-lived scoped cookie stop working without
// re-signing the whole deployment's JWT secret.
const SessionVersionClaim = "ver"

// SessionVersion reads the version claim. Tokens minted before the version
// column existed carry no claim and are treated as version 0, matching the
// migration default so upgrading a deployment does not sign everybody out.
func SessionVersion(claims jwt.MapClaims) int64 {
	if claims == nil {
		return 0
	}
	switch v := claims[SessionVersionClaim].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

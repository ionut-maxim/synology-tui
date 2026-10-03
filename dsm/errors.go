package dsm

import "fmt"

// Error is a DSM API failure ({"success": false, "error": {"code": N}}).
type Error struct {
	API    string
	Method string
	Code   int
}

func (e *Error) Error() string {
	msg, ok := authCodes[e.Code]
	if !ok || e.API != "SYNO.API.Auth" {
		msg, ok = commonCodes[e.Code]
	}
	if !ok {
		msg = "error"
	}
	return fmt.Sprintf("dsm: %s.%s: %s (code %d)", e.API, e.Method, msg, e.Code)
}

// SessionExpired reports whether logging in again would help.
func (e *Error) SessionExpired() bool {
	return e.Code == 106 || e.Code == 107 || e.Code == 119
}

// Codes shared by all APIs.
var commonCodes = map[int]string{
	100: "unknown error",
	101: "missing or invalid parameter",
	102: "API does not exist",
	103: "method does not exist",
	104: "version not supported",
	105: "permission denied",
	106: "session timed out",
	107: "session interrupted by a duplicate login",
	108: "upload failed",
	114: "missing or invalid parameter",
	117: "this NAS is managed by another server",
	119: "session not found",
	120: "invalid parameter",
}

// SYNO.API.Auth codes.
var authCodes = map[int]string{
	400: "no such account or incorrect password",
	401: "account disabled",
	402: "permission denied",
	403: "2-step verification code required",
	404: "2-step verification failed",
	406: "2-step verification is enforced for this account",
	407: "this IP is blocked",
	408: "password expired and can't be changed",
	409: "password expired",
	410: "password must be changed",
}

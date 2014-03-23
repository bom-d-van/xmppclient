package xmppclient

import (
	"strings"
)

// RemoveResourceFromJid returns the user@domain portion of a JID.
func RemoveResourceFromJid(jid string) string {
	slash := strings.Index(jid, "/")
	if slash != -1 {
		return jid[:slash]
	}
	return jid
}

func IsBareJid(jid string) bool {
	if jid == RemoveResourceFromJid(jid) {
		return true
	}
	return false
}

func GetLocalFromJID(jid string) string {
	return strings.Split(jid, "@")[0]
}

func SeparateJidAndResource(fullJid string) (bareJid, resource string) {
	arr := strings.Split(fullJid, "/")
	bareJid = arr[0]
	if len(arr) > 1 {
		resource = arr[1]
	}
	return
}

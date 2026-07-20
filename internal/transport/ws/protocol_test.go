package ws

import "testing"

func TestDecodeConnectAcceptsNewAndSavedCredentials(t *testing.T) {
	for _, raw := range []string{
		`{"type":"connect","port":22,"username":"admin","password":"secret","useSavedCredential":false,"remember":true,"cols":80,"rows":24}`,
		`{"type":"connect","port":2222,"username":"admin","password":"","useSavedCredential":true,"remember":false,"cols":120,"rows":40}`,
	} {
		message, err := decodeConnect([]byte(raw))
		if err != nil || message.Username != "admin" {
			t.Fatalf("message=%+v err=%v", message, err)
		}
	}
}

func TestDecodeConnectRejectsInvalidOrAmbiguousMessages(t *testing.T) {
	for _, raw := range []string{
		`{"type":"connect","port":0,"username":"admin","password":"secret","cols":80,"rows":24}`,
		`{"type":"connect","port":22,"username":"admin","password":"","cols":80,"rows":24}`,
		`{"type":"connect","port":22,"username":"admin","password":"secret","useSavedCredential":true,"cols":80,"rows":24}`,
		`{"type":"connect","port":22,"username":"admin","password":"secret","cols":1,"rows":24}`,
		`{"type":"connect","port":22,"username":"admin","password":"secret","cols":80,"rows":24,"extra":true}`,
		`{"type":"connect","port":22,"username":"admin","password":"secret","cols":80,"rows":24} {}`,
	} {
		if _, err := decodeConnect([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

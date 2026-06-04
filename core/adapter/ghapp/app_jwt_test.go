// core/adapter/ghapp/app_jwt_test.go
package ghapp_test

import (
	"testing"

	"agentic-delegator/core/adapter/ghapp"
)

func TestAppClient_zeroValueDoesntPanic(t *testing.T) {
	// Just instantiate — actual token minting needs a real PEM, which is
	// validated end-to-end in the staging smoke test.
	_ = ghapp.NewAppClient(ghapp.AppCreds{AppID: 1, PrivateKeyPEM: []byte("not-a-real-key")})
}

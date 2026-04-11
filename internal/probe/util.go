package probe

import (
	"crypto/rand"
	"fmt"
)

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

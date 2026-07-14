//go:build !windows

package cookies

import "fmt"

func dpapiUnprotect(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("Windows DPAPI decryption is only available on Windows")
}

//go:build windows

package cookies

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func dpapiUnprotect(encrypted []byte) ([]byte, error) {
	var in dataBlob
	if len(encrypted) > 0 {
		in.cbData = uint32(len(encrypted))
		in.pbData = &encrypted[0]
	}
	var out dataBlob
	result, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if result == 0 {
		return nil, fmt.Errorf("could not decrypt Windows DPAPI-protected data: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	if out.cbData == 0 {
		return []byte{}, nil
	}
	return append([]byte(nil), unsafe.Slice(out.pbData, int(out.cbData))...), nil
}

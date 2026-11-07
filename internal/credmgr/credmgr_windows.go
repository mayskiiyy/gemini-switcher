package credmgr

import (
	"encoding/json"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modAdvapi32   = syscall.NewLazyDLL("advapi32.dll")
	procCredReadW  = modAdvapi32.NewProc("CredReadW")
	procCredWriteW = modAdvapi32.NewProc("CredWriteW")
	procCredFree   = modAdvapi32.NewProc("CredFree")
)

const (
	TargetName       = "gemini:antigravity"
	CredTypeGeneric  = 1
	CredPersistLocal = 2
)

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        int64
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func ReadAntigravityCredential() (string, error) {
	targetPtr, err := syscall.UTF16PtrFromString(TargetName)
	if err != nil {
		return "", err
	}

	var pCred *winCredential
	r1, _, errSys := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(CredTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&pCred)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("CredReadW failed: %v", errSys)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pCred)))

	if pCred == nil || pCred.CredentialBlob == nil || pCred.CredentialBlobSize == 0 {
		return "", nil
	}

	blob := unsafe.Slice(pCred.CredentialBlob, pCred.CredentialBlobSize)
	return string(blob), nil
}

func WriteAntigravityCredential(blob string) error {
	targetPtr, err := syscall.UTF16PtrFromString(TargetName)
	if err != nil {
		return err
	}
	userNamePtr, err := syscall.UTF16PtrFromString("antigravity")
	if err != nil {
		return err
	}

	blobBytes := []byte(blob)
	var blobPtr *byte
	if len(blobBytes) > 0 {
		blobPtr = &blobBytes[0]
	}

	cred := winCredential{
		Flags:              0,
		Type:               CredTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(blobBytes)),
		CredentialBlob:     blobPtr,
		Persist:            CredPersistLocal,
		UserName:           userNamePtr,
	}

	r1, _, errSys := procCredWriteW.Call(
		uintptr(unsafe.Pointer(&cred)),
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("CredWriteW failed: %v", errSys)
	}
	return nil
}

func ParseSessionEmail(blob string) (string, error) {
	var parsed struct {
		IdTokenEmail string `json:"id_token_email"`
		Email        string `json:"email"`
	}
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		return "", err
	}
	if parsed.IdTokenEmail != "" {
		return parsed.IdTokenEmail, nil
	}
	if parsed.Email != "" {
		return parsed.Email, nil
	}
	return "", fmt.Errorf("no email found in credential blob")
}
